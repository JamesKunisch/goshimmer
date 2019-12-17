package autopeering

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"

	"github.com/iotaledger/autopeering-sim/discover"
	"github.com/iotaledger/autopeering-sim/logger"
	"github.com/iotaledger/autopeering-sim/peer"
	"github.com/iotaledger/autopeering-sim/peer/service"
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
	"outputPaths": ["./autopeering.log"],
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

var (
	db       peer.DB
	trans    *transport.TransportConn
	zLogger  *zap.SugaredLogger
	handlers []server.Handler
	conn     *net.UDPConn
)

func configureAP() {
	host := parameter.NodeConfig.GetString(CFG_ADDRESS)
	localhost := host
	apPort := strconv.Itoa(parameter.NodeConfig.GetInt(CFG_PORT))
	gossipPort := strconv.Itoa(parameter.NodeConfig.GetInt(gossip.GOSSIP_PORT))

	if host == "0.0.0.0" {
		host = getMyIP()
	}

	listenAddr := host + ":" + apPort
	gossipAddr := host + ":" + gossipPort

	zLogger = logger.NewLogger(defaultZLC, debugLevel)

	addr, err := net.ResolveUDPAddr("udp", localhost+":"+apPort)
	if err != nil {
		log.Fatalf("ResolveUDPAddr: %v", err)
	}
	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("ListenUDP: %v", err)
	}

	masterPeers := []*peer.Peer{}
	master, err := parseEntryNodes()
	if err != nil {
		log.Fatalf("Ignoring entry nodes: %v\n", err)
	} else if master != nil {
		masterPeers = master
	}

	// use the UDP connection for transport
	trans = transport.Conn(conn, func(network, address string) (net.Addr, error) { return net.ResolveUDPAddr(network, address) })

	// create a new local node
	db = peer.NewPersistentDB(zLogger.Named("db"))

	local.INSTANCE, err = peer.NewLocal("udp", listenAddr, db)
	if err != nil {
		log.Fatalf("ListenUDP: %v", err)
	}

	// add a service for the gossip
	if parameter.NodeConfig.GetBool(CFG_SELECTION) {
		local.INSTANCE.UpdateService(service.GossipKey, "tcp", gossipAddr)
	}

	Discovery = discover.New(local.INSTANCE, discover.Config{
		Log:         zLogger.Named("disc"),
		MasterPeers: masterPeers,
	})
	handlers = append([]server.Handler{}, Discovery)

	if parameter.NodeConfig.GetBool(CFG_SELECTION) {
		Selection = selection.New(local.INSTANCE, Discovery, selection.Config{
			Log: zLogger.Named("sel"),
			Param: &selection.Parameters{
				SaltLifetime:    selection.DefaultSaltLifetime,
				RequiredService: []service.Key{service.GossipKey},
			},
		})
		handlers = append(handlers, Selection)
	}
}

func start() {
	// start a server doing discovery and peering
	srv = server.Listen(local.INSTANCE, trans, zLogger.Named("srv"), handlers...)

	// start the discovery on that connection
	Discovery.Start(srv)

	// start the peering on that connection
	if parameter.NodeConfig.GetBool(CFG_SELECTION) {
		Selection.Start(srv)
		defer Selection.Close()
	}

	id := base64.StdEncoding.EncodeToString(local.INSTANCE.PublicKey())
	zLogger.Info("Discovery protocol started: ID=" + id + ", address=" + srv.LocalAddr())

	defer func() {
		_ = zLogger.Sync() // ignore the returned error
		trans.Close()
		db.Close()
		conn.Close()
		srv.Close()
		Discovery.Close()
	}()

	// Only for debug
	// go func() {
	// 	for t := range time.NewTicker(2 * time.Second).C {
	// 		_ = t
	// 		printReport(zLogger)
	// 	}
	// }()

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

// used only for debugging puropose
// func printReport(log *zap.SugaredLogger) {
// 	if Discovery == nil || Selection == nil {
// 		return
// 	}
// 	knownPeers := Discovery.GetVerifiedPeers()
// 	incoming := []*peer.Peer{}
// 	outgoing := []*peer.Peer{}
// 	if Selection != nil {
// 		incoming = Selection.GetIncomingNeighbors()
// 		outgoing = Selection.GetOutgoingNeighbors()
// 	}
// 	log.Info("Known peers:", len(knownPeers))
// 	log.Info("Chosen:", len(outgoing))
// 	log.Info("Accepted:", len(incoming))
// }
