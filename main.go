// main.go - Loop principal do jogo

// PRECISO FAZER FUNCAO QUE CRIA NOVO PLAYER QUE VEIO DO SERVER
// FAZER GOROUTINE DE 1 SEGUNDO QUE FICA PUXANDO A POSICAO DESSE NOVO PLAYER  
package main

import (
	"os"
	"time"
    "net/rpc"
	"log"
	"os/exec"
	"io"
	"sync"
)

type Player struct {
	ID    int
    Name  string
    PosX  int
    PosY  int
}

type CreatePlayerRequest struct {
    Name  string
    PosX  int
    PosY  int
	SequenceNumber uint64
}

type GetPlayerRequest struct {
    ID int
	SequenceNumber uint64
}

type PostPlayerPositionRequest struct {
	ID	 int
	PosX int
	PosY int
	SequenceNumber uint64
}

type RenderEvent struct {
    Player *Player
}

type RemoveEvent struct {
    Player *Player
}

var redrawCh chan struct{}

var oldPlayers []Player

var jogoMu sync.Mutex
var sendPosMu sync.Mutex
var createPlayerMu sync.Mutex
var getPlayerMu sync.Mutex
var listPlayersMu sync.Mutex

var sequenceNumber uint64
var seqMu sync.Mutex


func desenharSeguro() {
	select {
	case redrawCh <- struct{}{}:
	default:
	}
}

func sendPlayerPositionToServer(client *rpc.Client, player Player) bool{
	sendPosMu.Lock()
    defer sendPosMu.Unlock()

	seqMu.Lock()
	sequenceNumber++
	seqMu.Unlock()

	sendPlayerPosReq := PostPlayerPositionRequest{ID: player.ID, PosX: player.PosX, PosY: player.PosY, SequenceNumber: sequenceNumber}

	for attempt := 1; attempt <= 3; attempt++ {
		err := client.Call("PlayerService.UpdatePlayerPosition", &sendPlayerPosReq, nil)

		if err == nil {
			return true;
		}

		log.Printf("[WARN] Falha ao enviar posicao (seq=%d), tentativa %d/3: %v", sendPlayerPosReq.SequenceNumber, attempt, err)
	}
	
	log.Printf("[ERRO] Não foi possível enviar posição (seq=%d) após 3 tentativas", sendPlayerPosReq.SequenceNumber)

	sequenceNumber--

	return false
}

func criaPlayer(client *rpc.Client, player Player) Player {
	createPlayerMu.Lock()
    defer createPlayerMu.Unlock()

	seqMu.Lock()
    sequenceNumber++
    seq := sequenceNumber
    seqMu.Unlock()

	createReq := CreatePlayerRequest{PosX: player.PosX, PosY: player.PosY, Name: player.Name, SequenceNumber: seq}
    var newPlayer Player

	for attempt := 1; attempt <= 3; attempt++ {
		err := client.Call("PlayerService.CreatePlayer", &createReq, &newPlayer)

		if err == nil {
			log.Printf("Jogador criado (seq=%d): %+v\n", seq, newPlayer)
            return newPlayer
		}
		
		log.Printf("Falha ao criar jogador (seq=%d) tentativa %d/3 — %v",seq, attempt, err)
	}

	//Erro ao criar novo jogado --> retorna jogador vazio.
	return Player{}
}

func GetPlayerById(client *rpc.Client, playerID int) Player{
	getPlayerMu.Lock()
    defer getPlayerMu.Unlock()

	var fetched Player
    getReq := GetPlayerRequest{ID: playerID}
    err := client.Call("PlayerService.GetPlayer", &getReq, &fetched)
    if err != nil {
        log.Fatal("Erro ao buscar jogador:", err)
    }

	//Passar para main que chama
    log.Println("Jogador encontrado:", fetched)

	return fetched
}


func listAllPlayers(client *rpc.Client) []Player{
	listPlayersMu.Lock()
    defer listPlayersMu.Unlock()

	var allPlayers []Player

	for attempt := 1; attempt <= 3; attempt++ {
		err := client.Call("PlayerService.ListPlayers", &struct{}{}, &allPlayers)

		if err == nil {
			return allPlayers
		}

		log.Printf("Falha ao listar jogadores, tentativa %d/3 — %v", attempt, err)
	}

	// Nao foi possivel listar os players
	return nil
}


func main() {
	
	// INICIALIZA LOGS E OUTRO TERMINAL
	logFile, err := os.OpenFile("jogo.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
    if err != nil {
        log.Println("Erro ao abrir arquivo de log:", err)
        return
    }
    defer logFile.Close()
	// Escreve tanto no terminal principal quanto no log
    multiWriter := io.MultiWriter(os.Stdout, logFile)
    log.SetOutput(multiWriter)

    log.Println("Arquivo de log criado: jogo.log")

    // Abre um novo terminal que acompanha o log em tempo real
    cmd := exec.Command("cmd", "/c", "start", "cmd", "/k", "powershell Get-Content jogo.log -Wait")
    if err := cmd.Start(); err != nil {
        log.Println("Erro ao abrir terminal de log:", err)
    }
	
	// conecta no servidor
    serverAddr := "localhost:8932" // Ex: "localhost:1234"

    client, err := rpc.Dial("tcp", serverAddr)
    if err != nil {
        log.Fatal("Erro ao conectar:", err)
    }

	player := criaPlayer(client, Player{
    	PosX: 10,
    	PosY: 10,
    	Name: "Vicente",
	})

	// Inicializa a interface (termbox)
	interfaceIniciar()
	defer interfaceFinalizar()

	// Usa "mapa.txt" como arquivo padrão ou lê o primeiro argumento
	mapaFile := "mapa.txt"
	if len(os.Args) > 1 {
		mapaFile = os.Args[1]
	}

	// Inicializa o jogo
	jogo := jogoNovo()
	if err := jogoCarregarMapa(mapaFile, &jogo); err != nil {
		panic(err)
	}

	// Desenha o estado inicial do jogo
	interfaceDesenharJogo(&jogo)

	//Area que o nosso grupo produziu

	// Goroutine que faz a busca pela  lista de jogadores no servidor e atualiza a posição dos players online no mapa
	var playersOnline = make(map[int]*Player)
	var renderCh = make(chan RenderEvent, 32)
	var removeCh = make(chan RemoveEvent, 32)

	go func() {
	    ticker := time.NewTicker(200 * time.Millisecond)
	    defer ticker.Stop()

	    for range ticker.C {

	        newPlayers := listAllPlayers(client)

	        // Marca quem continua no jogo
	        activeIDs := make(map[int]bool)

	        for _, p := range newPlayers {

	            // Não atualiza o jogador local
	            if p.ID == player.ID {
	                continue
	            }

	            activeIDs[p.ID] = true

	            // Player já existe no mapa → apenas atualiza movimento
	            if localP, exists := playersOnline[p.ID]; exists {

	                // Se a posição mudou, atualiza no mapa
	                if localP.PosX != p.PosX || localP.PosY != p.PosY {

	                    // remove posição antiga
	                    removeCh <- RemoveEvent{Player: localP}

	                    // atualiza struct
	                    localP.PosX = p.PosX
	                    localP.PosY = p.PosY

	                    // desenha nova posição
	                    renderCh <- RenderEvent{Player: localP}
	                }

	            } else {
	                // Player é novo → adicionar
	                np := p // CÓPIA segura (evita pointer bug)
	                playersOnline[np.ID] = &np

	                log.Println("Novo player entrou:", np.Name)

	                renderCh <- RenderEvent{Player: &np}
	            }
	        }

	        // Remover players que saíram
	        for id, pl := range playersOnline {
            	if !activeIDs[id] {
            	    log.Println("Player saiu:", pl.Name)
            	    removeCh <- RemoveEvent{Player: pl}
            	    delete(playersOnline, id)
            	}
        	}
	    }
	}()

	//Goroutine responsável pelos canais de renderização e exclusão de players do mapa
	go func() {
    	for {
    	    select {
    	    case e := <-renderCh:
    	        jogoMu.Lock()
    	        renderizaPlayerOnline(&jogo, e.Player)
    	        jogoMu.Unlock()

    	    case e := <-removeCh:
    	        jogoMu.Lock()
    	        removePlayerDoMapa(&jogo, e.Player)
    	        jogoMu.Unlock()
    	    }
    	    desenharSeguro()
    	}
	}()




	//1° Goroutine
	//Insere concorrentemente uma Flor no mapa a cada 5 segundos (funcionalidade independente)
	florTick := make(chan struct{}, 1)

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			florTick <- struct{}{}
		}
	}()

	go func() {
		for range florTick {
			insereFlor(&jogo)
			desenharSeguro()
		}
	}()

	//2° Goroutine
	//Processa comandos do canal florCh
	florCh := make(chan FlorCmd)
	go func() {
		for cmd := range florCh {
			switch cmd.Tipo {
			case FlorConsumirAdj:
				if consomeFlorAdjacente(&jogo, cmd.X, cmd.Y) {
					jogo.StatusMsg = "Você comeu uma flor 🌸"
				} else {
					jogo.StatusMsg = "Nenhuma flor adjacente"
				}
				desenharSeguro()
			}
		}
	}()

	//3° Goroutine 
	// Cria um canal que, com uma lógica de semáforos, movimenta um inimigo por vez em um intervalo de 700 milisegundos
	// Onde controla o movimento do inimigo e o consumirFlor que ele também implementa
	// A comunicação está definida em 3 buffers, onde então, somente 4 inimigos podem se movimentar ao mesmo tempo
	inimigoTick := make(chan struct{}, 1)
	semInimigo := make(chan struct{}, 3)

	go func() {
		ticker := time.NewTicker(700 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			inimigoTick <- struct{}{}
		}
	}()

	go func() {
		for range inimigoTick {
			inimigos := listarInimigos(&jogo)
			for _, p := range inimigos {
				//funcao concorrente para andar o inimigo ao receber o tick de inimigo
				//maximo de 3 inimigos caminhando simultaneamente (limitado pelo semaforo) 
				// por tick do canal inimigoTick
				go func(px, py int) {
					semInimigo <- struct{}{}
					defer func() { <-semInimigo }()

					moverInimigo(&jogo, px, py)
					desenharSeguro()
				}(p.X, p.Y)

				time.Sleep(80 * time.Millisecond)
			}
		}
	}()

	// 4° Goroutine
	// Dementador aparece de 7 em 7 segundos para tentar comer um inimigo (como se fosse um buraco negro)
	// Onde ele atinge inimigos até 9x9 casas de distância do centro dele.
	// Caso um inimigo caia no range do dementador, ele não conseguirá se mover!

	dementadorTick := make(chan struct{}, 1)

	go func() {
		ticker := time.NewTicker(7 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			dementadorTick <- struct{}{}
		}
	}()


	go func() {
		for range dementadorTick {
			if x, y, ok := insereDementador(&jogo); ok {
				desenharSeguro()
				go func(px, py int) {
					time.Sleep(2 * time.Second)
					dementadorConsumir(&jogo, px, py)

					if py >= 0 && py < len(jogo.Mapa) && px >= 0 && px < len(jogo.Mapa[py]) {
						if jogo.Mapa[py][px] == Dementador {
							jogo.Mapa[py][px] = Vazio
						}
					}

					desenharSeguro()
				}(x, y)
			}
		}
	}()


	// Goroutine 5
	// Spawna um jardineiro que consome flores e vegetações presentes no mapa em um raio de 4 espaços

	jardineiroTick := make(chan struct{}, 1)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			jardineiroTick <- struct{}{}
		}
	}()

	go func() {
		for range jardineiroTick {
			if x, y, ok := insereJardineiro(&jogo); ok {
				desenharSeguro()

				go func(px, py int) {
					time.Sleep(3 * time.Second) // delay antes de agir

					jardineiroConsumir(&jogo, px, py)

					// remove o próprio Jardineiro, se ainda existir
					if py >= 0 && py < len(jogo.Mapa) && px >= 0 && px < len(jogo.Mapa[py]) {
						if jogo.Mapa[py][px] == Jardineiro {
							jogo.Mapa[py][px] = Vazio
						}
					}

					desenharSeguro()
				}(x, y)
			}
		}
	}()

	//Método seguro para atualizar o mapa do jogo concorrentemente
	redrawCh = make(chan struct{}, 1)

	go func() {
		for range redrawCh {
			// TEM QUE CHAMAR SERVER QUE RENDERIZA O JOGO
			sendPlayerPositionToServer(client, player)
			interfaceDesenharJogo(&jogo)
		}
	}()


	// Loop principal de entrada
	for {
		evento := interfaceLerEventoTeclado()

		if evento.Tipo == "interagir" {
			florCh <- FlorCmd{Tipo: FlorConsumirAdj, X: jogo.PosX, Y: jogo.PosY}
			continue
		}

		if continuar := personagemExecutarAcao(evento, &jogo); !continuar {
			break
		}
		player.PosX = jogo.PosX
		player.PosY = jogo.PosY
		sendPlayerPositionToServer(client, player)
		desenharSeguro()
	}
}