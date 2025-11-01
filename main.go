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
}

type GetPlayerRequest struct {
    ID int
}

type PostPlayerPositionRequest struct {
	ID	 int
	PosX int
	PosY int
}

var redrawCh chan struct{}

var oldPlayers []Player

func desenharSeguro() {
	select {
	case redrawCh <- struct{}{}:
	default:
	}
}

func sendPlayerPositionToServer(client *rpc.Client, player Player) bool{
	sendPlayerPosReq := PostPlayerPositionRequest{ID: player.ID, PosX: player.PosX, PosY: player.PosY}
	err := client.Call("PlayerService.UpdatePlayerPosition", &sendPlayerPosReq, nil)
	if err != nil {
		log.Fatal("Erro ao atualizar posicao do jogador:", err)
		return false;
	}

	return true
}

func criaPlayer(client *rpc.Client, player Player) Player {
	createReq := CreatePlayerRequest{PosX: player.PosX, PosY: player.PosY, Name: player.Name}
    var newPlayer Player
    err := client.Call("PlayerService.CreatePlayer", &createReq, &newPlayer)
    if err != nil {
        log.Fatal("Erro ao criar jogador:", err)
    }

	//Passar para main que chama
    log.Println("Jogador criado:", newPlayer)

	return newPlayer
}

func GetPlayerById(client *rpc.Client, playerID int) Player{
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
	var allPlayers []Player
    err := client.Call("PlayerService.ListPlayers", &struct{}{}, &allPlayers)
    if err != nil {
        log.Fatal("Erro ao listar jogadores:", err)
    }

	return allPlayers
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

	var playersOnline = make(map[int]*Player)

	go func() {
	    ticker := time.NewTicker(200 * time.Millisecond)
	    defer ticker.Stop()
	
	    for range ticker.C {
		
	        // 1. Busca lista do servidor
	        newPlayers := listAllPlayers(client)
		
	        // 2. Cria mapa para marcar quem está ativo no servidor
	        activeIDs := make(map[int]bool)
		
	        for _, p := range newPlayers {
			
	            activeIDs[p.ID] = true // marca como ativo
			
	            if p.ID == player.ID {
	                continue // não atualiza você mesmo
	            }
			
	            if localP, exists := playersOnline[p.ID]; exists {
	                // 3. Se já existe, só atualiza pos
	                localP.PosX = p.PosX
	                localP.PosY = p.PosY
	            } else {
	                // 4. Se for novo → adiciona no mapa e renderiza
	                np := p // cópia segura
	                playersOnline[p.ID] = &np
				
	                log.Println("🎉 Novo player entrou:", np.Name)
	                renderizaPlayerOnline(&jogo, &np)
	            }
	        }
		
	        // 5. Remove players que saíram (não estão mais no servidor)
	        for id := range playersOnline {
	            if !activeIDs[id] {
	                log.Println("👋 Player saiu:", playersOnline[id].Name)
	                removePlayerDoMapa(&jogo, playersOnline[id]) // você pode implementar isso ou eu faço
	                delete(playersOnline, id)
	            }
	        }
		
	        // 6. Redesenha sem flood
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