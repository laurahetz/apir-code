package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/si-co/vpir-code/lib/client"
	"github.com/si-co/vpir-code/lib/database"
	"github.com/si-co/vpir-code/lib/proto"
	"github.com/si-co/vpir-code/lib/utils"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/encoding/gzip"
)

const (
	configEnvKey = "VPIR_CONFIG"

	defaultConfigFile = "../config.toml"
)

type localClient struct {
	ctx         context.Context
	callOptions []grpc.CallOption
	connections map[string]*grpc.ClientConn

	prg        *utils.PRGReader
	config     *utils.Config
	flags      *flags
	dbInfo     *database.Info
	vpirClient client.Client
}

// TODO: remove useless flags
type flags struct {
	// experiments flag
	repetitions    int
	elemBitSize    int
	bitsToRetrieve int

	// scheme flags
	scheme string

	// flags for point queries
	id string

	// flags for complex queries
	target    string
	fromStart int
	fromEnd   int
	and       bool
	avg       bool
}

func parseFlags() *flags {
	f := new(flags)

	// experiments flags
	flag.IntVar(&f.repetitions, "repetitions", -1, "experiment repetitions")
	flag.IntVar(&f.elemBitSize, "elemBitSize", -1, "bit size of element, in which block lengtht is specified")
	flag.IntVar(&f.bitsToRetrieve, "bitsToRetrieve", -1, "number of bits to retrieve in experiment")

	// scheme flags
	flag.StringVar(&f.scheme, "scheme", "", "scheme to use")

	// flag for point queries
	flag.StringVar(&f.id, "id", "", "id of key to retrieve")

	// flag for complex queries
	flag.StringVar(&f.target, "target", "", "target for complex query")
	flag.IntVar(&f.fromStart, "from-start", 0, "from start parameter for complex query")
	flag.IntVar(&f.fromEnd, "from-end", 0, "from end parameter for complex query")
	flag.BoolVar(&f.and, "and", false, "and clause for complex query")
	flag.BoolVar(&f.avg, "avg", false, "avg clause for complex query")

	flag.Parse()

	return f
}

func newLocalClient() *localClient {
	// initialize local client
	lc := &localClient{
		ctx: context.Background(),
		callOptions: []grpc.CallOption{
			grpc.UseCompressor(gzip.Name),
			grpc.MaxCallRecvMsgSize(1024 * 1024 * 1024),
			grpc.MaxCallSendMsgSize(1024 * 1024 * 1024),
		},
		prg:   utils.RandomPRG(),
		flags: parseFlags(),
	}

	// set logs to stdout
	log.SetOutput(os.Stdout)
	log.SetPrefix(fmt.Sprintf("[Client] "))

	// load configs
	configPath := os.Getenv(configEnvKey)
	if configPath == "" {
		configPath = defaultConfigFile
	}

	config, err := utils.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("could not load the config file: %v", err)
	}
	lc.config = config

	return lc
}

func main() {
	lc := newLocalClient()

	err := lc.connectToServers()
	defer lc.closeConnections()

	if err != nil {
		log.Fatal(err)
	}

	_, err = lc.exec()
	if err != nil {
		log.Fatal(err)
	}

}

func (lc *localClient) exec() (string, error) {
	// get and store db info.
	lc.retrieveDBInfo()

	// start correct client
	switch lc.flags.scheme {
	case "pir-classic", "pir-merkle":
		lc.vpirClient = client.NewPIR(lc.prg, lc.dbInfo)
		lc.retrievePointPIR()
	default:
		return "", xerrors.Errorf("wrong scheme: %s", lc.flags.scheme)
	}
	// case "pointPIR", "pointVPIR":
	// 	lc.vpirClient = client.NewPIR(lc.prg, lc.dbInfo)

	// 	// get id
	// 	if lc.flags.id == "" {
	// 		var id string
	// 		fmt.Print("please enter the id: ")
	// 		fmt.Scanln(&id)
	// 		if id == "" {
	// 			log.Fatal("id not provided")
	// 		}
	// 		lc.flags.id = id
	// 	}

	// 	// retrieve the key corresponding to the id
	// 	return lc.retrieveKeyGivenId(lc.flags.id)
	// case "complexPIR":
	// 	lc.vpirClient = client.NewPredicatePIR(lc.prg, lc.dbInfo)
	// 	out, err := lc.retrieveComplexQuery()
	// 	if err != nil {
	// 		return "", err
	// 	}
	// 	return strconv.FormatUint(uint64(out), 10), nil
	// case "complexVPIR":
	// 	lc.vpirClient = client.NewPredicateAPIR(lc.prg, lc.dbInfo)
	// 	out, err := lc.retrieveComplexQuery()
	// 	if err != nil {
	// 		return "", err
	// 	}
	// 	return strconv.FormatUint(uint64(out), 10), nil
	// default:
	// 	return "", xerrors.Errorf("wrong scheme: %s", lc.flags.scheme)
	// }

	return "", nil
}

func (lc *localClient) retrievePointPIR() {
	numTotalBlocks := lc.dbInfo.NumRows * lc.dbInfo.NumColumns
	numRetrieveBlocks := bitsToBlocks(lc.dbInfo.BlockSize, lc.flags.elemBitSize, lc.flags.bitsToRetrieve)

	var startIndex int
	queryByte := make([]byte, 4)
	for j := 0; j < lc.flags.repetitions; j++ {
		log.Printf("start repetition %d out of %d", j+1, lc.flags.repetitions)

		// pick a random block index to start the retrieval
		startIndex = rand.Intn(numTotalBlocks - numRetrieveBlocks)

		for i := 0; i < numRetrieveBlocks; i++ {
			binary.BigEndian.PutUint32(queryByte, uint32(startIndex+i))
			queries, err := lc.vpirClient.QueryBytes(queryByte, len(lc.connections))
			if err != nil {
				log.Fatal("error when executing query:", err)
			}
			log.Printf("done with queries computation")

			// send queries to servers
			answers := lc.runQueries(queries)

			// reconstruct
			_, err = lc.vpirClient.ReconstructBytes(answers)
			if err != nil {
				log.Fatal("error during reconstruction:", err)
			}
			log.Printf("done with block reconstruction")
		}
	}
}

func (lc *localClient) connectToServers() error {
	// load servers certificates
	creds, err := utils.LoadServersCertificates()
	if err != nil {
		return xerrors.Errorf("could not load servers certificates: %v", err)
	}

	// connect to servers and store connections
	lc.connections = make(map[string]*grpc.ClientConn)
	for _, s := range lc.config.Addresses {
		conn, err := connectToServer(creds, s)
		if err != nil {
			return xerrors.Errorf("failed to connect: %v", err)
		}

		lc.connections[s] = conn
	}

	return nil
}

func (lc *localClient) closeConnections() {
	for _, conn := range lc.connections {
		err := conn.Close()
		if err != nil {
			log.Printf("failed to close conn: %v", err)
		}
	}
}

func (lc *localClient) retrieveDBInfo() {
	subCtx, cancel := context.WithTimeout(lc.ctx, time.Hour)
	defer cancel()

	wg := sync.WaitGroup{}
	resCh := make(chan *database.Info, len(lc.connections))
	for _, conn := range lc.connections {
		wg.Add(1)
		go func(conn *grpc.ClientConn) {
			resCh <- dbInfo(subCtx, conn, lc.callOptions)
			wg.Done()
		}(conn)
	}
	wg.Wait()
	close(resCh)

	dbInfo := make([]*database.Info, 0)
	for i := range resCh {
		dbInfo = append(dbInfo, i)
	}

	// check if db info are all equal before returning
	if !equalDBInfo(dbInfo) {
		log.Fatal("got different database info from servers")
	}

	log.Printf("databaseInfo: %#v", dbInfo[0])

	lc.dbInfo = dbInfo[0]
}

func dbInfo(ctx context.Context, conn *grpc.ClientConn, opts []grpc.CallOption) *database.Info {
	c := proto.NewVPIRClient(conn)
	q := &proto.DatabaseInfoRequest{}
	answer, err := c.DatabaseInfo(ctx, q, opts...)
	if err != nil {
		log.Fatalf("could not send database info request to %s: %v",
			conn.Target(), err)
	}
	log.Printf("sent databaseInfo request to %s", conn.Target())

	dbInfo := &database.Info{
		NumRows:    int(answer.GetNumRows()),
		NumColumns: int(answer.GetNumColumns()),
		BlockSize:  int(answer.GetBlockLength()),
		PIRType:    answer.GetPirType(),
		Merkle:     &database.Merkle{Root: answer.GetRoot(), ProofLen: int(answer.GetProofLen())},
	}

	return dbInfo
}

func equalDBInfo(info []*database.Info) bool {
	for i := range info {
		if info[0].NumRows != info[i].NumRows ||
			info[0].NumColumns != info[i].NumColumns ||
			info[0].BlockSize != info[i].BlockSize {
			return false
		}
	}

	return true
}

func connectToServer(creds credentials.TransportCredentials, address string) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	conn, err := grpc.DialContext(ctx, address,
		grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		return nil, xerrors.Errorf("did not connect to %s: %v", address, err)
	}

	log.Println("connected to server", address)

	return conn, nil
}

// Converts number of bits to retrieve into the number of db blocks
func bitsToBlocks(blockSize, elemSize, numBits int) int {
	return int(math.Ceil(float64(numBits) / float64(blockSize*elemSize)))
}

func (lc *localClient) runQueries(queries [][]byte) [][]byte {
	subCtx, cancel := context.WithTimeout(lc.ctx, time.Hour)
	defer cancel()

	wg := sync.WaitGroup{}
	resCh := make(chan []byte, len(lc.connections))
	j := 0
	for _, conn := range lc.connections {
		wg.Add(1)
		go func(j int, conn *grpc.ClientConn) {
			resCh <- queryServer(subCtx, conn, lc.callOptions, queries[j])
			wg.Done()
		}(j, conn)
		j++
	}
	wg.Wait()
	close(resCh)

	// combinate answers of all the servers
	q := make([][]byte, 0)
	for v := range resCh {
		q = append(q, v)
	}

	return q
}

func queryServer(ctx context.Context, conn *grpc.ClientConn, opts []grpc.CallOption, query []byte) []byte {
	c := proto.NewVPIRClient(conn)
	q := &proto.QueryRequest{Query: query}
	answer, err := c.Query(ctx, q, opts...)
	if err != nil {
		log.Fatalf("could not query %s: %v",
			conn.Target(), err)
	}
	log.Printf("sent query to %s", conn.Target())
	log.Printf("query size in bytes %d", len(query))

	return answer.GetAnswer()
}
