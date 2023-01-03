package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/libp2p/go-libp2p/p2p/muxer/mplex"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/mr-tron/base58"

	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/websocket"
	ma "github.com/multiformats/go-multiaddr"
)

func main(){
	Start(PeerId())
}

var nameTopic = "my_topic"
var ctx context.Context
var h host.Host



func PeerId()string{
	
	var privKey crypto.PrivKey
	var err error
		privKey, _, err = crypto.GenerateKeyPair(crypto.Ed25519, 0)
		if err != nil {
			panic(err)
		}
		bytes, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			panic(err)
		}
		enc := base58.Encode(bytes)
		fmt.Print("enc: ", enc)
		return enc
}

func Start(peerId string)   {
	var privKey crypto.PrivKey
	privateKeyBytes, err := base58.Decode(peerId)
	if err != nil {
		panic(err)
	}
	privKey, err = crypto.UnmarshalPrivateKey(privateKeyBytes)
	if err != nil {
		panic(err)
	}
	

connectionManager, err := connmgr.NewConnManager(
	100, // Lowwater
	400, // HighWater,
	connmgr.WithGracePeriod(time.Minute),
)
if err != nil {
	panic(err)
}

var connection = libp2p.ChainOptions(
	libp2p.ConnectionManager(connectionManager),
	libp2p.NATPortMap(),
	libp2p.EnableNATService(),
)

security := libp2p.ChainOptions(
	libp2p.Security(libp2ptls.ID, libp2ptls.New),
	libp2p.Security(noise.ID, noise.New),
)

transports := libp2p.ChainOptions(
	libp2p.Transport(tcp.NewTCPTransport),
	libp2p.Transport(websocket.New),
)

muxers := libp2p.ChainOptions(
	libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
	libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
)
	
var port string = "5002"
var relayMultiaddrs []ma.Multiaddr
relayMultiaddr, _ := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/5001/p2p/12D3KooWDLvrgs6Y2DN35ue6ntibrzxzdk5BCSW4DaogDcdpGoXE")
relayMultiaddrs = append(relayMultiaddrs, relayMultiaddr)

relay := libp2p.ChainOptions()
var peerInfos []peer.AddrInfo

	for _, relayAddr := range relayMultiaddrs{
		peerInfo, _ := peer.AddrInfoFromP2pAddr(relayAddr)
		peerInfos = append(peerInfos, *peerInfo)
	}
 

	

	h, err = libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/"+port),
		libp2p.Ping(false),
		connection,
		security,
		transports,
		muxers,
		relay,
		
	)
	if err != nil {
		panic(err)
	}


	ctx = context.Background()
	// ctxch <- ctx
	// hch <- h

	 getMessage := make(chan string)
	anyConnectedchan := make(chan bool)
//---------------------------------------------------------
// Pubsub
//---------------------------------------------------------
go DiscoverPeers(ctx, h, anyConnectedchan)
// fmt.Println(<-complete)


ps, err := pubsub.NewGossipSub(ctx, h)
if err != nil {
	panic(err)
}

topic, err := ps.Join(nameTopic)
	if err != nil {
		panic(err)
	}
	
	go streamConsoleTo(ctx, topic, getMessage)
	// fmt.Println("getMessage: ", getMessage)

	sub, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	fmt.Print(sub)
	printMessagesFrom(ctx , sub)


	if err := h.Close(); err != nil {
        fmt.Println("host close of Error ", err)
    }
	id := "I'm" + h.ID().String()
	addr := h.Addrs()

	fmt.Println(id, addr)
	// fmt.Println("pubsub: ", ps)


	
	select {}
	
}



func initDHT(ctx context.Context, h host.Host) *dht.IpfsDHT {
	// Start a DHT, for use in peer discovery. We can't just make a new DHT
	// client because we want each peer to maintain its own local copy of the
	// DHT, so that the bootstrapping node of the DHT can go down without
	// inhibiting future peer discovery.
	kademliaDHT, err := dht.New(ctx, h)
	if err != nil {
		panic(err)
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		panic(err)
	}
	var wg sync.WaitGroup

	for _, peerAddr := range dht.DefaultBootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.Connect(ctx, *peerinfo); err != nil {
				fmt.Println("Bootstrap warning:", err)
			}
		}()
	}
	wg.Wait()

	return kademliaDHT
}

func DiscoverPeers(ctx context.Context, h host.Host, anyConnectedchan chan bool) {
	
	kademliaDHT := initDHT(ctx, h)
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, nameTopic)

	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	  
	for !anyConnected {
		fmt.Println("Searching for peers...")
		fmt.Println(h.ID())
		peerChan, err := routingDiscovery.FindPeers(ctx, nameTopic)
		if err != nil {
			panic(err)
		}
		for peer := range peerChan {
			if peer.ID == h.ID() {
				continue // No self connection
			}
			err := h.Connect(ctx, peer)
			if err != nil {
				fmt.Println("Failed connecting to ", peer.ID.Pretty(), ", error:", err)
			} else {
				fmt.Println("Connected to:", peer.ID.Pretty())
				anyConnected = true
				anyConnectedchan <- anyConnected
			}
		}
	}
	fmt.Print("Peer discovery complete \n")
	// complete <- "Peer discovery complete \n"
	// defer close(complete)
}



func streamConsoleTo(ctx context.Context, topic *pubsub.Topic, getMessage chan string) {
	reader := bufio.NewReader(os.Stdin)
	for {
		s, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}
		if err := topic.Publish(ctx, []byte(s)); err != nil {
			fmt.Println("### Publish error:", err)
		}
		getMessage <- s
	}
}

func printMessagesFrom(ctx context.Context, sub *pubsub.Subscription) {
	for {
		m, err := sub.Next(ctx)
		if err != nil {
			panic(err)
		}
		fmt.Println(m.ReceivedFrom, ": ", string(m.Message.Data))
	}
}













