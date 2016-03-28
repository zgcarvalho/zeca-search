package search

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand"
	"os"

	"github.com/gonum/stat"
	zmq "github.com/pebbe/zmq4"
)

func RunMaster(conf Config) {
	// Le as probabilidades da regra
	pk := ReadProbRule(conf.Rules.Input)

	// cria o emissor que envia as probabilidades para toda a rede na porta A
	sender, _ := zmq.NewSocket(zmq.PUSH)
	defer sender.Close()
	sender.Bind("tcp://*:" + conf.Dist.PortA)

	// cria o receptor que recebe os individuos de toda a rede na porta B
	receiver, _ := zmq.NewSocket(zmq.PULL)
	defer receiver.Close()
	receiver.Bind("tcp://*:" + conf.Dist.PortB)

	// a população do DistEDA será de apenas vencedores dos torneios locais,
	// realizados nos slaves. Portanto, população é igual população/n_torneio
	var pop []Individual
	pop = make([]Individual, conf.EDA.Population/conf.EDA.Tournament)

	// popFitness := make([]float64, conf.EDA.Population/conf.EDA.Tournament)
	// popQ3 := make([]float64, conf.EDA.Population/conf.EDA.Tournament)
	popScore := make([]float64, conf.EDA.Population/conf.EDA.Tournament)

	// cria um arquivo de log onde serão salvas as estatisticas por geração, como
	// média e variância do Q3
	fstat, err := os.Create("log")
	if err != nil {
		panic(err)
	}
	defer fstat.Close()

	// Enviar as configurações para os slaves para que eles não precisem ler um
	// arquivo de configuração
	// Quando os slaves responderem OK o master pode começar a distribuir regras e
	// receber os vencedores
	// TODO automatizar essa comunicação com os slaves e remover a espera abaixo

	// os slaves demoram um pouco para inicializar pois precisam acessar o DB e
	// carregar os dados. O master precisa esperar os slaves estarem prontos. Por
	// enquanto, o sinal de inicio é dado manualmente (TODO -> pensar numa forma
	// automática)
	fmt.Print("Press Enter when the workers are ready: ")
	var line string
	fmt.Scanln(&line)
	fmt.Println("Sending tasks to workers...")

	// Inicio do processamento
	fmt.Println("RUNNING MASTER")
	// para cada geracao
	for g := 0; g < conf.EDA.Generations; g++ {
		fmt.Println("GERACAO", g)

		if g != 0 {
			pk.Update(pop)
		}

		// Criar as probabilidades para serem enviadas no formato JSON.
		// Será enviado o ID (PID) = hash da probabilidade, o número da geração e as
		// probabilidades
		write := new(bytes.Buffer)
		encoder := gob.NewEncoder(write)
		prob := &Probabilities{PID: rand.Uint32(), Generation: g, Data: pk}
		encoder.Encode(prob)

		// tmp, err := json.Marshal(pk)
		// if err != nil {
		// 	fmt.Println("Error: Creating json 1", err)
		// }
		// pid := adler32.Checksum(tmp)
		// prob := &Probabilities{PID: pid, Generation: g, Data: pk}
		// b, err := json.Marshal(prob)
		// if err != nil {
		// 	fmt.Println("Error: Creating json 2", err)
		// }

		// Para cada individuo que precisará retornar deve ser emitida uma
		// probabilidade. Uma goroutine fica emitindo probabilidades que vão sendo
		// capturados pelos slaves que após o torneio, devolvem o vencedor
		// go func(b *[]byte) {
		// 	for i := 0; i < len(pop); i++ {
		// 		sender.Send(string(*b), 0)
		// 	}
		// }(&b)

		go func(w *bytes.Buffer) {
			for i := 0; i < len(pop); i++ {
				sender.SendBytes(w.Bytes(), 0)
			}
		}(write)

		// Capta os individuos vencedores gerados pelos slaves
		for i := 0; i < len(pop); {
			m, err := receiver.RecvBytes(0)
			if err == nil {
				// json.Unmarshal([]byte(m), &pop[i])
				read := bytes.NewBuffer(m)
				decoder := gob.NewDecoder(read)
				decoder.Decode(&pop[i])
				// Checa pelo ID da probabilidade se o individuo vencedor que chegou foi
				// gerado pela última probabilidade que foi emitida
				if prob.PID == pop[i].PID {
					// fmt.Printf("Individuo id: %d rid: %d g: %d, score: %f\n", g*len(pop)+i, pop[i].PID, pop[i].Generation, pop[i].Fitness)
					// popFitness[i] = pop[i].Fitness
					// popQ3[i] = pop[i].Q3
					popScore[i] = pop[i].Score
					i++
				} else {
					fmt.Println(prob.PID, pop[i].PID)
				}

			} else {
				fmt.Println(err)
			}
		}

		// IMPORTANTE
		// TODO criar um mecanismo para contornar falhas nos nós

		// // imprimir e as estatisticas// salva as probabilidades a cada geração
		// err := ioutil.WriteFile(conf.EDA.OutputProbs+"_g"+strconv.Itoa(g), []byte(p.String()), 0644)
		// if err != nil {
		// 	fmt.Println("Erro gravar as probabilidades")
		// 	fmt.Println(p)
		// }

		//  imprimir e as estatisticas
		// meanFit, stdFit := stat.MeanStdDev(popFitness, nil)
		// meanQ3, stdQ3 := stat.MeanStdDev(popQ3, nil)
		meanScore, stdScore := stat.MeanStdDev(popScore, nil)
		// fstat.WriteString(fmt.Sprintf("G: %d, Mean Score: %.5f, StdDev Score: %.5f, Mean: %.5f, StdDev: %.5f, Mean Q3: %.5f, StdDev Q3: %.5f, \n", g, meanScore, stdScore, meanFit, stdFit, meanQ3, stdQ3))
		// fmt.Printf("G: %d, Mean Score: %.5f, StdDev Score: %.5f, Mean: %.5f, StdDev: %.5f, Mean Q3: %.5f, StdDev Q3: %.5f, \n", g, meanScore, stdScore, meanFit, stdFit, meanQ3, stdQ3)
		fstat.WriteString(fmt.Sprintf("G: %d, Mean Score: %.5f, StdDev Score: %.5f\n", g, meanScore, stdScore))
		fmt.Printf("G: %d, Mean Score: %.5f, StdDev Score: %.5f\n", g, meanScore, stdScore)
	}
}
