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
This constructor receives coordinates of chain, message handler, manager config as parameters.

Config consists of:

type Config struct {
	Port      uint16
	Network   string
	StorePath string
}

The Network field can include tcp, udp and mock (custom) and enter the port number to use in the Port field.
Network 필드에는 tcp, udp, mock(custom)이 포함될수 있고 Port 필드에는 사용할 포트 번호를 입력합니다.
In the StorePath field, enter the path you want to save the DB file.
StorePath 필드에는 DB파일을 저장할 위치를 입력합니다</code></pre>

## Manager

### RegisterEventHandler(eh EventHandler)

<pre><code>This function is used to register handlers that require events at the time of connecting and disconnecting peers.
이 함수는 peer가 연결되고 끊어지는 시점의 이벤트를 필요로하는 handler를 등록할때 사용합니다.
The parameters of the RegisterEventHandler must implement EventHandler.
RegisterEventHandler의 파라미터는 EventHandler를 구현해야 합니다.</code></pre>

### StartManage()

<pre><code>This function performs the functions required for peer manager to run.
</code></pre>

### AddNode(addr string) error
<pre><code></code></pre>
### BroadCast(m message.Message)
<pre><code></code></pre>
### NodeList() []string
<pre><code></code></pre>



## Peer

### net.Conn
<pre><code></code></pre>
### Send(m message.Message)
<pre><code></code></pre>
### PingTime() time.Duration
<pre><code></code></pre>
### SetPingTime(t time.Duration)
<pre><code></code></pre>
### ConnectedTime() int64
<pre><code></code></pre>
### IsClose() bool
<pre><code></code></pre>


<pre><code></code></pre>

## EventHandler

### PeerConnected(p Peer)

<pre><code></code></pre>

### PeerClosed(p Peer)

<pre><code></code></pre>

