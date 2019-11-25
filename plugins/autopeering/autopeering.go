package autopeering

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/iotaledger/autopeering-sim/discover"
	"github.com/iotaledger/autopeering-sim/logger"
	"github.com/iotaledger/autopeering-sim/peer"
	"github.com/iotaledger/autopeering-sim/selection"
	"github.com/iotaledger/autopeering-sim/server"
	"github.com/iotaledger/autopeering-sim/transport"
	"github.com/iotaledger/goshimmer/plugins/autopeering/local"
	"github.com/iotaledger/goshimmer/plugins/gossip"
	"github.com/iotaledger/hive.go/parameter"
	"go.uber.org/zap"
)

var (
	debugLevel = "info"
	close      = make(chan struct{}, 1)
	srv        *server.Server
	Discovery  *discover.Protocol
	Selection  *selection.Protocol
)

const defaultZLC = `{
	"level": "info",
	"development": false,
	"outputPaths": ["stdout"],
	"errorOutputPaths": ["stderr"],
	"encoding": "console",
	"encoderConfig": {
	  "timeKey": "ts",
	  "levelKey": "level",
	  "nameKey": "logger",
	  "callerKey": "caller",
	  "messageKey": "msg",
	  "stacktraceKey": "stacktrace",
	  "lineEnding": "",
	  "levelEncoder": "",
	  "timeEncoder": "iso8601",
	  "durationEncoder": "",
	  "callerEncoder": ""
	}
  }`

func start() {
	var (
		err error
	)

	host := parameter.NodeConfig.GetString(CFG_ADDRESS)
	localhost := host
	apPort := strconv.Itoa(parameter.NodeConfig.GetInt(CFG_PORT))
	gossipPort := strconv.Itoa(parameter.NodeConfig.GetInt(gossip.GOSSIP_PORT))
	if host == "0.0.0.0" {
		host = getMyIP()
	}
	listenAddr := host + ":" + apPort
	gossipAddr := host + ":" + gossipPort

	logger := logger.NewLogger(defaultZLC, debugLevel)

	defer func() { _ = logger.Sync() }() // ignore the returned error

	addr, err := net.ResolveUDPAddr("udp", localhost+":"+apPort)
	if err != nil {
		log.Fatalf("ResolveUDPAddr: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("ListenUDP: %v", err)
	}
	defer conn.Close()

	masterPeers := []*peer.Peer{}
	master, err := parseEntryNodes()
	if err != nil {
		log.Fatalf("Ignoring entry nodes: %v\n", err)
	} else if master != nil {
		masterPeers = master
	}

	// use the UDP connection for transport
	trans := transport.Conn(conn, func(network, address string) (net.Addr, error) { return net.ResolveUDPAddr(network, address) })
	defer trans.Close()

	// create a new local node
	db := peer.NewPersistentDB(logger.Named("db"))
	defer db.Close()
	local.INSTANCE, err = peer.NewLocal(db)
	if err != nil {
		log.Fatalf("ListenUDP: %v", err)
	}
	// add a service for the peering
	local.INSTANCE.Services()["peering"] = peer.NetworkAddress{Network: "udp", Address: listenAddr}
	// add a service for the gossip
	local.INSTANCE.Services()["gossip"] = peer.NetworkAddress{Network: "tcp", Address: gossipAddr}

	Discovery = discover.New(local.INSTANCE, discover.Config{
		Log:         logger.Named("disc"),
		MasterPeers: masterPeers,
	})
	Selection = selection.New(local.INSTANCE, Discovery, selection.Config{
		Log:          logger.Named("sel"),
		SaltLifetime: selection.DefaultSaltLifetime,
	})

	// start a server doing discovery and peering
	srv = server.Listen(local.INSTANCE, trans, logger.Named("srv"), Discovery, Selection)
	defer srv.Close()

	// start the discovery on that connection
	Discovery.Start(srv)
	defer Discovery.Close()

	// start the peering on that connection
	Selection.Start(srv)
	defer Selection.Close()

	id := base64.StdEncoding.EncodeToString(local.INSTANCE.PublicKey())
	a, b, _ := net.SplitHostPort(srv.LocalAddr())
	logger.Info("Discovery protocol started: ID="+id+", address="+srv.LocalAddr(), a, b)

	go func() {
		for t := range time.NewTicker(2 * time.Second).C {
			_ = t
			printReport(logger)
		}
	}()

	<-close
}

func getMyIP() string {
	url := "https://api.ipify.org?format=text"
	resp, err := http.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s", ip)
}

func printReport(log *zap.SugaredLogger) {
	if Discovery == nil || Selection == nil {
		return
	}
	knownPeers := Discovery.GetVerifiedPeers()
	incoming := Selection.GetIncomingNeighbors()
	outgoing := Selection.GetOutgoingNeighbors()
	log.Info("Known peers:", len(knownPeers))
	log.Info("Chosen:", len(outgoing), outgoing)
	log.Info("Accepted:", len(incoming), incoming)
}