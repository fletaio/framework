#Summary

Peer is a connection management library.
The main function is to search for peers and propagate around them. It also keeps a certain number of peers and avoids network bias
피어를 검색하고 주변에 전파하는 역할이 주 기능입니다. 또한 일정 수 의 피어를 유지하도록 하며, 망 쏠림 현상을 기피합니다.

# Functions

<pre><code>NewManager(ChainCoord *common.Coordinate, mh *message.Handler, cfg Config) (Manager, error)

type Manager interface {
	RegisterEventHandler(eh EventHandler)
	StartManage()
	EnforceConnect()
	AddNode(addr string) error
	BroadCast(m message.Message)
	NodeList() []string
	ConnectedList() []string
	TargetCast(addr string, m message.Message) error
	ExceptCast(addr string, m message.Message)
}

type Peer interface {
	net.Conn
	Send(m message.Message)
	PingTime() time.Duration
	SetPingTime(t time.Duration)
	ConnectedTime() int64
	IsClose() bool
}

type EventHandler interface {
	PeerConnected(p Peer)
	PeerClosed(p Peer)
}</code></pre>

## NewManager(ChainCoord *common.Coordinate, mh *message.Handler, cfg Config) (Manager, error)

<pre><code>NewManager is the constructor of the peer manager.
This constructor receives StorePath and BanEvilScore as parameters.

Config consists of:

type Config struct {
	StorePath string
	BanEvilScore uint16
}

The StorePath field is the path to the DB file and the BanEvilleScore field is the base score for ban the peer.
StorePath 필드는 DB 파일의 경로이며 BanEvilScore 필드는 피어를 ban하는 기준 점수입니다.</code></pre>

## Manager

### RegisterEventHandler(eh EventHandler)

<pre><code>This function is used to register handlers that require events at the time of connecting and disconnecting peers.
이 함수는 peer가 연결되고 끊어지는 시점의 이벤트를 필요로하는 handler를 등록할때 사용합니다.
The parameters of the RegisterEventHandler must implement EventHandler.
RegisterEventHandler의 파라미터는 EventHandler를 구현해야 합니다.</code></pre>

### StartManage()

<pre><code>This function initiates the functions that peer managers need to communicate with outside and manage their peers.
이 함수는 피어 매니저가 외부와 통신을 하고, 피어들을 관리하는데 필요한 기능들을 시작합니다.</code></pre>

### AddNode(addr string) error
<pre><code>This function is used to add nodes directly.
직접적으로 Node를 추가할 때 사용하는 함수 입니다.</code></pre>

### BroadCast(m message.Message)
<pre><code>This function used to transfer messages to all connected nodes.
연결되어 있는 모든 노드에 message를 전송할 때 사용하는 함수입니다.</code></pre>

### NodeList() []string
<pre><code>This function returns a peer list with a connected record even once.
한번이라도 연결된 기록이 있는 peer 목록을 리턴하는 함수 입니다.
It can be removed from the list by evil score.
evil score에 의해 목록에서 제거될 수 있습니다.</code></pre>

### ConnectedList() []string
<pre><code>This function returns the list of currently connected peers.
현재 연결되어 있는 peer의 목록을 리턴하는 함수 입니다.
This returns the list of peers connected to the calling time of the function, so must check the connection before using it.
함수의 호출시점에 연결되어 있는 peer의 목록을 리턴 하므로, 사용시에 disconnection을 확인 후에 사용해야 합니다.</code></pre>

### TargetCast(addr string, m message.Message) error
<pre><code>Send a message by specifying the target peer as a parameter.
목표 피어를 파라미터로 지정하여 message를 전송합니다.
Returns an ErrNotFoundPer error if not found the target peer.
목표한 피어가 없을 경우 ErrNotFoundPeer 에러를 리턴합니다.</code></pre>

### ExceptCast(addr string, m message.Message)
<pre><code>Specify a specific peer to send a message to all connected peers except for that peer.
특정 피어를 지정하여 해당 피어만 제외하고 연결된 모든 피어에 message를 전송합니다.</code></pre>

## Peer

### net.Conn
<pre><code>This is included to use the peer structure as a "net.Conn".
Peer 구조체를 net.Conn 처럼 사용하기 위해 포함합니다.</code></pre>

### Send(m message.Message)
<pre><code>Send message to peer.</code></pre>

### PingTime() time.Duration
<pre><code>Returns the ping time of this peer.</code></pre>

### SetPingTime(t time.Duration)
<pre><code>Sets the ping time of this peer.</code></pre>

### ConnectedTime() int64
<pre><code>Returns the created time of this peer</code></pre>

### IsClose() bool
<pre><code>Returns the peer close status</code></pre>

## EventHandler
<pre><code>Interface used to perform additional tasks externally when a peer is connected and disconnected.
외부에서 피어가 연결된 시점과 연결이 끊긴 시점에 추가적인 동작을 수행 할 수 있도록 도와주는 인터페이스 입니다.</code></pre>

### PeerConnected(p Peer)

<pre><code>Called after the connection is established and the data is ready to read and send
연결이 수립되고 데이터를 읽을 준비를 완료한 후에 호출됩니다.</code></pre>

### PeerDisconnected(p Peer)

<pre><code>This function is called just before net.Conn closes.
net.Conn이 close 되기 바로 직전에 호출됩니다.
It can't send or receive any additional data, but it can see the instantly available data to net.Conn.
추가적으로 데이터를 주고받을순 없지만 net.Conn에서 즉각적으로 사용할 수 있는 데이터를 열람 할 수 있습니다.</code></pre>

