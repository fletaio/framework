#Summary

Fleta's router is a connection library that supports communication between multiple chains.
플래타의 라우터는 multy chain간의 통신을 지원해주는 인터넷 중계 라이브러리입니다.
The incoming packet is distributed based on the coordinates of each chain to support independent communication between the chains.
하나의 connection으로 들어오는 packet을 각 chain의 coordinate로 구분하여 분배하는 과정을 추가하여 chain간의 독립적인 통신을 지원합니다.

#examples

<pre><code>cd := common.Coordinate{}
r := router.NewRouter("tcp", 3000) // 라우터 생성(network string, port uint16)
r.AddListen(&cd)                   // listen 과정

wg := sync.WaitGroup{} // wg : 테스트 코드를 모두 수행할때 까지 wait시킬 WaitGroup
wg.Add(2)              // wg : 전송하고 받는 2가지 케이스를 wait

go func() {
	conn, err := r.Accept(&cd) // 연결시도가 들어오면 conn을 리턴
	if err != nil {            // conn을 받은 이후 에러가 발생 할 경우 패닉처리
		panic(err)
	}
	conn.Write([]byte("hello fleta")) // conn에 test messgae 전송
	wg.Done()                         // wg : 전송하는 케이스 완료
}()

go func() {
	otherRouter := router.NewRouter("tcp", 3000) // 다른 라우터를 생성

	go func() {
		conn, err := otherRouter.Accept(&cd) // 연결이 수립되면 conn을 리턴
		if err != nil {                      // conn을 받은 이후 에러가 발생 할 경우 패닉처리
			panic(err)
		}
		bs := make([]byte, 2048)
		n, err := conn.Read(bs) // message를 전송받을 대기중 맞 연결된 receiver에서 send/flush를 하면 []byte를 받음
		if err != nil {         // data를 받은 이후 에러가 발생 할 경우 패닉처리
			panic(err)
		}
		log.Println(string(bs[:n])) // 전송받은 message를 출력
		wg.Done()                   // wg : 받는 케이스 완료
	}()

	err := otherRouter.Request("127.0.0.1:3000", &cd) // 로컬 호스트에 연결
	if err != nil {                                   // receiver를 받은 이후 에러가 발생 할 경우 패닉처리
		panic(err)
	}
}()

wg.Wait() // wg : 2가지 케이스가 완료 될 때까지 대기</code></pre>

## Result

<pre><code>hello fleta</code></pre>

# Functions

<pre><code>NewRouter(Network string, port uint16) Router

type Router interface {
	AddListen(ChainCoord *common.Coordinate) error
	Request(addrStr string, ChainCoord *common.Coordinate) error
	Accept(ChainCoord *common.Coordinate) (net.Conn, error)
}</code></pre>

## NewRouter(Network string, port uint16) Router

<pre><code>NewRouter is the constructor of the router.
This constructor receives the network (tcp, udp, etc...) and port number as parameters.</code></pre>

## AddListen(ChainCoord *common.Coordinate) error

<pre><code>AddListen is a feature that receives physical connections.
물리적 연결을 listen 하는 함수입니다.
It is waiting for a connection request from another router, and when it occurs, create a connection to create a read-standby state.
다른 Router에서의 연결 요청을 대기 하고 있으며 요청이 발생 했을 경우 connection을 생성하여 읽기 대기 상태를 만듭니다.
AddListen accepts duplicate port requests because they can be called from multiple chains.
멸티 체인에서 AddListen을 호출할 수 있으므로 중복된 포트요청을 인정합니다.
If the AddListen function uses duplicate ports, retain existing features and do not perform any additional actions.
중복된 포트로 AddListen함수를 호출 하면 기존의 listen을 유지하며 AddListen함수에서는 추가적인 동작을 하지 않습니다.</code></pre>

## Request(addrStr string, ChainCoord *common.Coordinate) error

<pre><code>Request is a function of trying physical connections.
물리적 연결을 시도하는 함수 입니다.
Request a connection to another router waiting to make a physical connection. Once the physical connection is established, the router requests "accept" with the coordinates it receives during the handshake process.
AddListen 하고 있는 다른 Router에 물리연결 주소로 연결을 요청하여 물리적 연결이 수립되면 handshake과정을 거처 기제된 coordinate를 구분자로 listen하고 있는 router에 Accept요청을 합니다.
If this is already physically connected to the same router, the router request "Accept" using the coordinate values passed from the handshake process
같은 router에 이미 물리연결이 되어있는 경우 handshake과정만 새로 거처 router에 Accept요청을 합니다.</code></pre>

## Accept(ChainCoord *common.Coordinate) (net.Conn, error)

<pre><code>This function is waits for connection with another router.
When a request is received or request accepted, the "net.Conn" is returned to the "Accept" requestor using the coordinates registered in the handshake process.
It is a 1:1 pair with the address and coordinate of the Dial.</code></pre>
