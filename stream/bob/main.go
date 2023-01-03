package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	dht "github.com/libp2p/go-libp2p-kad-dht"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/libp2p/go-libp2p/p2p/muxer/mplex"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/mr-tron/base58"

	// "github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/libp2p/go-libp2p/p2p/transport/websocket"
	// ma "github.com/multiformats/go-multiaddr"
)

func main(){
	Start(PeerId())
}



var protocolStream = "/chat/stream/1.0.0"
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

func handleStream(stream network.Stream) {
	fmt.Print("Got a new stream!")

	// Create a buffer stream for non blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	go readData(rw)
	go writeData(rw)

	// 'stream' will stay open until you close it (or the other side closes it).
}

func readData(rw *bufio.ReadWriter) {
	for {
		str, err := rw.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from buffer")
			panic(err)
		}

		if str == "" {
			return
		}
		if str != "\n" {
			// Green console colour: 	\x1b[32m
			// Reset console colour: 	\x1b[0m
			fmt.Printf("\x1b[32m%s\x1b[0m> ", str)
		}

	}
}

func writeData(rw *bufio.ReadWriter) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading from stdin")
			panic(err)
		}

		_, err = rw.WriteString(fmt.Sprintf("%s\n", sendData))
		if err != nil {
			fmt.Println("Error writing to buffer")
			panic(err)
		}
		err = rw.Flush()
		if err != nil {
			fmt.Println("Error flushing buffer")
			panic(err)
		}
	}
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
// var relayMultiaddrs []ma.Multiaddr
// relayMultiaddr, _ := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/5001/p2p/12D3KooWDLvrgs6Y2DN35ue6ntibrzxzdk5BCSW4DaogDcdpGoXE")
// relayMultiaddrs = append(relayMultiaddrs, relayMultiaddr)

// relay := libp2p.ChainOptions()
// var peerInfos []peer.AddrInfo

// 	for _, relayAddr := range relayMultiaddrs{
// 		peerInfo, _ := peer.AddrInfoFromP2pAddr(relayAddr)
// 		peerInfos = append(peerInfos, *peerInfo)
// 	}
 

	

	h, err = libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/"+port),
		libp2p.Ping(false),
		connection,
		security,
		transports,
		muxers,
		// relay,
		
	)
	if err != nil {
		panic(err)
	}


	ctx = context.Background()
	// ctxch <- ctx
	// hch <- h

	
	 complete := make(chan string)

	 h.SetStreamHandler(protocol.ID(protocolStream), handleStream)
//---------------------------------------------------------
// Stream
//---------------------------------------------------------
go DiscoverPeers(complete)
fmt.Println(<-complete)


	id := "I'm" + h.ID().String()
	addr := h.Addrs()

	fmt.Println(id, addr)


	
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

func DiscoverPeers(complete chan string) {
	
	kademliaDHT := initDHT(ctx, h)
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)
	dutil.Advertise(ctx, routingDiscovery, protocolStream)

	// Look for others who have announced and attempt to connect to them
	anyConnected := false
	for !anyConnected {
		fmt.Println("Searching for peers...")
		fmt.Println(h.ID())
		peerChan, err := routingDiscovery.FindPeers(ctx, protocolStream)
		if err != nil {
			panic(err)
		}
		for peer := range peerChan {
			if peer.ID == h.ID() {
				continue
			}
			fmt.Println("Found peer:", peer)
	
			fmt.Println("Connecting to:", peer)
			stream, err := h.NewStream(ctx, peer.ID, protocol.ID(protocolStream))
	
			if err != nil {
				fmt.Println("Connection failed:", err)
				continue
			} else {
				rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
				anyConnected = true
				go writeData(rw)
				go readData(rw)
			}
	
			fmt.Println("Connected to:", peer)
		}
	}
	fmt.Print("Peer discovery complete \n")
	complete <- "Peer discovery complete \n"
	defer close(complete)
}








