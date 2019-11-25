package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
	"github.com/sjwhitworth/golearn/base"
	"github.com/sjwhitworth/golearn/evaluation"
	"github.com/sjwhitworth/golearn/knn"
	"github.com/davecgh/go-spew/spew"
	"github.com/joho/godotenv"
	"encoding/csv"
)

// Block represents each 'item' in the blockchain
type Block struct {
	Index     int
	Timestamp string
	BPM       int
	Hash      string
	PrevHash  string
	Validator string
	Transactions []Transaction
}

type Miner struct {
	HashMiner string
	wallet int
	Index int
	Success int
	Faults int
}

type Transaction struct {
	Id int
	SourceAddr string
	DestAddr string
	Value int
	GasPrice float64
	Age int
}

// Blockchain is a series of validated Blocks
var Blockchain []Block
var tempBlocks []Block

var transactionPool []Transaction
// candidateBlocks handles incoming blocks for validation
var candidateBlocks = make(chan Block)

// announcements broadcasts winning validator to all nodes
var announcements = make(chan string)

var mutex = &sync.Mutex{}
var blockCount int
type ByAge []Transaction
type ByGas []Transaction
var countBlock map[string]int
var start time.Time
// validators keeps track of open validators and balances
var validators = make(map[string]int)

func main() {
	blockCount =0
	countBlock = make(map[string]int)
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}
	start = time.Now()
	createTransactions()
	// create genesis block
	t := time.Now()
	genesisBlock := Block{}
	var tran []Transaction
	genesisBlock = Block{0, t.String(), 0, calculateBlockHash(genesisBlock), "", "", tran}
	spew.Dump(genesisBlock)
	Blockchain = append(Blockchain, genesisBlock)

	httpPort := os.Getenv("PORT")

	// start TCP and serve TCP server
	server, err := net.Listen("tcp", ":"+httpPort)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("HTTP Server Listening on port :", httpPort)
	defer server.Close()

	go func() {
		for candidate := range candidateBlocks {
			mutex.Lock()
			tempBlocks = append(tempBlocks, candidate)
			mutex.Unlock()
		}
	}()

	go func() {
		for {
			pickWinner()
		}
	}()

	for {
		conn, err := server.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn)
	}
}

func createTransactions(){
	csvFile, _ := os.Open("transaction.csv")
    reader := csv.NewReader(bufio.NewReader(csvFile))
    index := 1
    for {
        line, error := reader.Read()
        if error == io.EOF {
            break
        } else if error != nil {
            log.Fatal(error)
		}
		val,_ :=  strconv.Atoi(line[7])
		age,_ := strconv.Atoi(line[11])
		gas, _ := strconv.ParseFloat(line[9], 4)
        transactionPool = append(transactionPool, Transaction{
            Id: index,
			SourceAddr: line[5],
			DestAddr: line[6],
			Value: val,
			GasPrice: gas,
			Age: age,
			},
		)
		index++
	}
}


// pickWinner creates a lottery pool of validators and chooses the validator who gets to forge a block to the blockchain
// by random selecting from the pool, weighted by amount of tokens staked
func pickWinner() {
	time.Sleep(30 * time.Second)
	mutex.Lock()
	temp := tempBlocks
	mutex.Unlock()

	lotteryPool := []string{}
	if len(temp) > 0 {

		// slightly modified traditional proof of stake algorithm
		// from all validators who submitted a block, weight them by the number of staked tokens
		// in traditional proof of stake, validators can participate without submitting a block to be forged
	
		for _, block := range temp {
			
					lotteryPool = append(lotteryPool, block.Validator)
	
		}
		// var m map[string]float64
		// m= make(map[string]float64)
		// var n int
		// n=1
		var maxAcc float64
		maxAcc =0
		var lotteryWinner string
		algos := [3]string{"euclidean", "manhattan", "cosine"}
		distances := [2]string{"linear", "kdtree"}
		for _, miner := range lotteryPool {
			acc, err := getAccuracy(algos[rand.Intn(len(algos))],distances[rand.Intn(len(distances))], rand.Intn(4) + 1)
			fmt.Println(miner,acc)
			if err == nil {
				if acc>maxAcc {
					maxAcc = acc
					lotteryWinner = miner
				}
			}

		}
		
		// randomly pick winner from lottery pool
		// s := rand.NewSource(time.Now().Unix())
		// r := rand.New(s)
		// lotteryWinner := lotteryPool[r.Intn(len(lotteryPool))]
		
		// add block of winner to blockchain and let all the other nodes know
		transactionsSelected := getBestTransactions()
		blockCount++
		value, ok := countBlock[lotteryWinner]
		if ok {
			countBlock[lotteryWinner]  = value + 1
		} else {
			countBlock[lotteryWinner] = 1
		}
		fmt.Println("The number of blocks mined by each of the validator is ",countBlock)
		for _, block := range temp {
			if block.Validator == lotteryWinner {
				mutex.Lock()
				block.Transactions = transactionsSelected
				Blockchain = append(Blockchain, block)
				mutex.Unlock()
				for _ = range validators {
					announcements <- "\nwinning validator: " + lotteryWinner + "\n"
				}
				break
			}
		}
		elapsed := time.Since(start)
		if blockCount%5 == 0{		
		log.Printf("It took %s to mine the block", elapsed)
		 }
	}
	mutex.Lock()
	tempBlocks = []Block{}
	mutex.Unlock()
}
// #########
func (a ByAge) Len() int  { return len(a) }
func (a ByAge) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByAge) Less(i, j int) bool { return a[i].Age > a[j].Age }
// #########

// #########
func (a ByGas) Len() int  { return len(a) }
func (a ByGas) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByGas) Less(i, j int) bool { return a[i].GasPrice > a[j].GasPrice }
// #########

// func RemoveIndex(s []int, index int) []int {
//     return append(s[:index], s[index+1:]...)
// }

func getBestTransactions() []Transaction {
	var selectT []Transaction
	sort.Sort(ByAge(transactionPool))
	i := 0
	in :=0
	for i<2 {
		valid := isTransactionValid(transactionPool[in])
		if valid == true {
			selectT = append(selectT, transactionPool[in])
			i++
		}
		in++
	}
	transactionPool = transactionPool[in:]
	i =0
	in =0
	
	// transactionPool = RemoveIndex(transactionPool,0)
	// transactionPool = RemoveIndex(transactionPool,1)
	// transactionPool = RemoveIndex(transactionPool,2)

	sort.Sort(ByGas(transactionPool))
	for i <2 {
		valid := isTransactionValid(transactionPool[in])
		if valid == true {
			selectT = append(selectT, transactionPool[in])
			i++
		}
		in++
	}
	transactionPool = transactionPool[in:]
	return selectT
}
func handleConn(conn net.Conn) {
	defer conn.Close()

	go func() {
		for {
			msg := <-announcements
			io.WriteString(conn, msg)
		}
	}()
	// validator address
	var address string

	// allow user to allocate number of tokens to stake
	// the greater the number of tokens, the greater chance to forging a new block
	io.WriteString(conn, "Enter wallet balance:")
	scanBalance := bufio.NewScanner(conn)
	for scanBalance.Scan() {
		balance, err := strconv.Atoi(scanBalance.Text())
		if err != nil {
			log.Printf("%v not a number: %v", scanBalance.Text(), err)
			return
		}
		t := time.Now()
		address = calculateHash(t.String())
		validators[address] = balance
		fmt.Println(validators)
		break
	}

	io.WriteString(conn, "\nEnter the transaction amount")

	scanBPM := bufio.NewScanner(conn)

	go func() {
		for {
			// take in BPM from stdin and add it to blockchain after conducting necessary validation
			for scanBPM.Scan() {
				bpm, err := strconv.Atoi(scanBPM.Text())
				// if malicious party tries to mutate the chain with a bad input, delete them as a validator and they lose their staked tokens
				if err != nil {
					log.Printf("%v not a number: %v", scanBPM.Text(), err)
					delete(validators, address)
					conn.Close()
				}
				var tran []Transaction
				mutex.Lock()
				oldLastIndex := Blockchain[len(Blockchain)-1]
				mutex.Unlock()
				
				// create newBlock for consideration to be forged
				newBlock, err := generateBlock(oldLastIndex, bpm, address,tran)
				if err != nil {
					log.Println(err)
					continue
				}
				if isBlockValid(newBlock, oldLastIndex) {
					candidateBlocks <- newBlock
				}
				io.WriteString(conn, "\nEnter the transaction amount")
				
			}
		}
	}()

	// simulate receiving broadcast
	for {
		time.Sleep(time.Minute)
		mutex.Lock()
		output, err := json.Marshal(Blockchain)
		mutex.Unlock()
		if err != nil {
			log.Fatal(err)
		}
		io.WriteString(conn, string(output)+"\n")
	}

}

func isTransactionValid(transaction Transaction) bool {
	if transaction.Id <=0  || transaction.Value <=0 || len(transaction.SourceAddr)==0 || len(transaction.DestAddr)==0 || transaction.GasPrice<0 || transaction.Age<0 {
		return false
	}
	/* get miner block also
	if balance - transaction.Value < 0 {
		miner.Penalty = miner.Penalty + 1
		return false
	}	
	*/
	
	return true
}


// isBlockValid makes sure block is valid by checking index
// and comparing the hash of the previous block
func isBlockValid(newBlock, oldBlock Block) bool {
	if oldBlock.Index+1 != newBlock.Index {
		return false
	}

	if oldBlock.Hash != newBlock.PrevHash {
		return false
	}

	if calculateBlockHash(newBlock) != newBlock.Hash {
		return false
	}

	return true
}

// SHA256 hasing
// calculateHash is a simple SHA256 hashing function
func calculateHash(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

//calculateBlockHash returns the hash of all block information
func calculateBlockHash(block Block) string {
	record := string(block.Index) + block.Timestamp + string(block.BPM) + block.PrevHash
	return calculateHash(record)
}

// generateBlock creates a new block using previous block's hash
func generateBlock(oldBlock Block, BPM int, address string, transaction []Transaction) (Block, error) {

	var newBlock Block

	t := time.Now()
	newBlock.Index = oldBlock.Index + 1
	newBlock.Timestamp = t.String()
	newBlock.BPM = BPM
	newBlock.PrevHash = oldBlock.Hash
	newBlock.Hash = calculateBlockHash(newBlock)
	newBlock.Validator = address
	newBlock.Transactions = transaction

	return newBlock, nil
}
func getAccuracy( dist string, algo string, k int) (float64, error) {
	datasets:= [3]string{"iris.csv","articles.csv","mnist_train.csv"}
	num_blocks:= len(Blockchain)
	difficulty:= 1
	if num_blocks>10{
		difficulty= 2
	}
	if num_blocks>20{
		difficulty=3
	}
	fmt.Println("Dataset:",datasets[difficulty-1])
	rawData, err := base.ParseCSVToInstances(datasets[difficulty-1], true)
	if err != nil {
		panic(err)
	}

	//Initialises a new KNN classifier
	cls := knn.NewKnnClassifier(dist, algo, k)

	//Do a training-test split
	trainData, testData := base.InstancesTrainTestSplit(rawData, 0.50)
	cls.Fit(trainData)

	//Calculates the Euclidean distance and returns the most popular label
	predictions, err := cls.Predict(testData)
	if err != nil {
		return 0, err
	}

	// Prints precision/recall metrics
	confusionMat, err := evaluation.GetConfusionMatrix(testData, predictions)
	if err != nil {
		return 0, err
	}
	return evaluation.GetAccuracy(confusionMat), nil
}