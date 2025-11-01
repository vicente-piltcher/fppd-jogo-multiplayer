// jogo.go - Funções para manipular os elementos do jogo, como carregar o mapa e mover o personagem
package main

import (
	"bufio"
	"math/rand"
	"os"
	"time"
)

// Elemento representa qualquer objeto do mapa (parede, personagem, vegetação, etc)
type Elemento struct {
	simbolo   rune
	cor       Cor
	corFundo  Cor
	tangivel  bool // Indica se o elemento bloqueia passagem
}

// Jogo contém o estado atual do jogo
type Jogo struct {
	Mapa            [][]Elemento // grade 2D representando o mapa
	PosX, PosY      int          // posição atual do personagem
	UltimoVisitado  Elemento     // elemento que estava na posição do personagem antes de mover
	StatusMsg       string       // mensagem para a barra de status
}

type FlorCmdType int

type FlorCmd struct {
	Tipo FlorCmdType 
	X, Y int
}	

type Pos struct{ 
	X, Y int 
}

const (
	FlorConsumirAdj FlorCmdType = iota	
)

// Elementos visuais do jogo
var (
	Personagem 	= Elemento{'☺', CorCinzaEscuro, CorPadrao, true}
	Inimigo    	= Elemento{'☠', CorVermelho, CorPadrao, true}
	Jardineiro  = Elemento{'※', CorVermelho, CorPadrao, true}
	Parede     	= Elemento{'▤', CorParede, CorFundoParede, true}
	Vegetacao  	= Elemento{'♣', CorVerde, CorPadrao, false}
	Vazio      	= Elemento{' ', CorPadrao, CorPadrao, false}
	Flor       	= Elemento{'✿', CorVerde, CorPadrao, false}
	Dementador	= Elemento{'⊙', CorVermelho, CorPadrao, true}
)

// Cria e retorna uma nova instância do jogo
func jogoNovo() Jogo {
	// O ultimo elemento visitado é inicializado como vazio
	// pois o jogo começa com o personagem em uma posição vazia
	return Jogo{UltimoVisitado: Vazio}
}

// Lê um arquivo texto linha por linha e constrói o mapa do jogo
func jogoCarregarMapa(nome string, jogo *Jogo) error {
	arq, err := os.Open(nome)
	if err != nil {
		return err
	}
	defer arq.Close()

	scanner := bufio.NewScanner(arq)
	y := 0
	for scanner.Scan() {
		linha := scanner.Text()
		var linhaElems []Elemento
		for x, ch := range linha {
			e := Vazio
			switch ch {
			case Parede.simbolo:
				e = Parede
			case Inimigo.simbolo:
				e = Inimigo
			case Vegetacao.simbolo:
				e = Vegetacao
			case Personagem.simbolo:
				jogo.PosX, jogo.PosY = x, y // registra a posição inicial do personagem
			}
			linhaElems = append(linhaElems, e)
		}
		jogo.Mapa = append(jogo.Mapa, linhaElems)
		y++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

// Verifica se o personagem pode se mover para a posição (x, y)
func jogoPodeMoverPara(jogo *Jogo, x, y int) bool {
	// Verifica se a coordenada Y está dentro dos limites verticais do mapa
	if y < 0 || y >= len(jogo.Mapa) {
		return false
	}

	// Verifica se a coordenada X está dentro dos limites horizontais do mapa
	if x < 0 || x >= len(jogo.Mapa[y]) {
		return false
	}

	// Verifica se o elemento de destino é tangível (bloqueia passagem)
	if jogo.Mapa[y][x].tangivel {
		return false
	}

	// Pode mover para a posição
	return true
}

// Move um elemento para a nova posição
func jogoMoverElemento(jogo *Jogo, x, y, dx, dy int) {
	nx, ny := x+dx, y+dy

	// Obtem elemento atual na posição
	elemento := jogo.Mapa[y][x] // guarda o conteúdo atual da posição

	jogo.Mapa[y][x] = jogo.UltimoVisitado     // restaura o conteúdo anterior
	jogo.UltimoVisitado = jogo.Mapa[ny][nx]   // guarda o conteúdo atual da nova posição
	jogo.Mapa[ny][nx] = elemento              // move o elemento
}

func insereFlor(jogo *Jogo) {
	if len(jogo.Mapa) == 0 {
		return
	}

	rand.Seed(time.Now().UnixNano())

	for {
		y := rand.Intn(len(jogo.Mapa))
		x := rand.Intn(len(jogo.Mapa[y]))

		if jogo.Mapa[y][x] == Vazio {
			jogo.Mapa[y][x] = Flor
			break
		}
	}
}

func listarInimigos(jogo *Jogo) []Pos {
	var v []Pos
	for y := range jogo.Mapa {
		for x := range jogo.Mapa[y] {
			if jogo.Mapa[y][x] == Inimigo {
				v = append(v, Pos{X: x, Y: y})
			}
		}
	}
	return v
}

func moverInimigo(jogo *Jogo, x, y int) {
	if y < 0 || y >= len(jogo.Mapa) || x < 0 || x >= len(jogo.Mapa[y]) {
		return
	}
	if jogo.Mapa[y][x] != Inimigo { 
		return 
	}

	if existeDementadorProximo(jogo, x, y, 4) { // range 4 (9x9 área)
		return
	}

	if consomeFlorAdjacente(jogo, x, y) {
		return // se consumiu flor, termina a ação neste tick
	}

	dirs := [][2]int{{0,-1}, {0,1}, {-1,0}, {1,0}}

	for tries := 0; tries < 8; tries++ {
		d := dirs[rand.Intn(len(dirs))]
		nx, ny := x+d[0], y+d[1]
		
		if ny < 0 || ny >= len(jogo.Mapa) {
			continue
		}

		if nx < 0 || nx >= len(jogo.Mapa[ny]) {
			continue
		}

		if jogo.Mapa[ny][nx] != Vazio {
			continue
		}

		jogo.Mapa[y][x] = Vazio
		jogo.Mapa[ny][nx] = Inimigo
		return
	}
}

func consomeFlorAdjacente(jogo *Jogo, px, py int) bool {
	deltas := [][2]int{{0,-1}, {0,1}, {-1,0}, {1,0}}

	for _, d := range deltas {
		x := px + d[0]
		y := py + d[1]

		if y < 0 || y >= len(jogo.Mapa) { continue }

		if x < 0 || x >= len(jogo.Mapa[y]) { continue }

		if jogo.Mapa[y][x] == Flor {
			jogo.Mapa[y][x] = Vazio
			return true
		}
	}

	return false
}

func insereDementador(jogo *Jogo) (int, int, bool) {
	if len(jogo.Mapa) == 0 {
		return 0, 0, false
	}

	rand.Seed(time.Now().UnixNano())
	for {
		y := rand.Intn(len(jogo.Mapa))
		x := rand.Intn(len(jogo.Mapa[y]))
		if jogo.Mapa[y][x] == Vazio {
			jogo.Mapa[y][x] = Dementador
			return x, y, true
		}
	}
}

func insereJardineiro(jogo *Jogo) (int, int, bool) {
	if len(jogo.Mapa) == 0 {
		return 0, 0, false
	}

	rand.Seed(time.Now().UnixNano())

	for {
		y := rand.Intn(len(jogo.Mapa))
		x := rand.Intn(len(jogo.Mapa[y]))

		if jogo.Mapa[y][x] == Vazio {
			jogo.Mapa[y][x] = Jardineiro
			return x, y, true
		}
	}
}

func dementadorConsumir(jogo *Jogo, cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x := cx + dx
			y := cy + dy
			if y < 0 || y >= len(jogo.Mapa) { continue }
			if x < 0 || x >= len(jogo.Mapa[y]) { continue }
			if jogo.Mapa[y][x] == Inimigo {
				jogo.Mapa[y][x] = Vazio
			}
		}
	}
}

func existeDementadorProximo(jogo *Jogo, px, py, rangeSize int) bool {
	for dy := -rangeSize; dy <= rangeSize; dy++ {
		for dx := -rangeSize; dx <= rangeSize; dx++ {
			x := px + dx
			y := py + dy

			if y < 0 || y >= len(jogo.Mapa) {
				continue
			}
			if x < 0 || x >= len(jogo.Mapa[y]) {
				continue
			}

			if jogo.Mapa[y][x] == Dementador {
				return true
			}
		}
	}
	return false
}

func jardineiroConsumir(jogo *Jogo, cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x := cx + dx
			y := cy + dy

			if y < 0 || y >= len(jogo.Mapa) {
				continue
			}
			if x < 0 || x >= len(jogo.Mapa[y]) {
				continue
			}

			if jogo.Mapa[y][x] == Flor || jogo.Mapa[y][x] == Vegetacao {
				jogo.Mapa[y][x] = Vazio
			}
		}
	}
}

func renderizaPlayerOnline(jogo *Jogo, player *Player) {
    // Limpa posição anterior se válida
    for y := range jogo.Mapa {
        for x := range jogo.Mapa[y] {
            if jogo.Mapa[y][x] == Personagem && (x != player.PosX || y != player.PosY) {
                jogo.Mapa[y][x] = Vazio
            }
        }
    }

    // Desenha nova posição
    jogo.Mapa[player.PosY][player.PosX] = Personagem
}

func removePlayerDoMapa(jogo *Jogo, p *Player) {
    if p.PosY >= 0 && p.PosY < len(jogo.Mapa) &&
       p.PosX >= 0 && p.PosX < len(jogo.Mapa[p.PosY]) {
        jogo.Mapa[p.PosY][p.PosX] = Vazio
    }
}




