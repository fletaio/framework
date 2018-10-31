#Summary

플래타의 라우터는 multy chain간의 통신을 지원해주는 인터넷 중계 라이브러리입니다.
하나의 connection으로 들어오는 packet을 각 chain의 coordinate로 구분하여 분배하는 과정을 추가하여 chain간의 독립적인 통신을 지원합니다.

#examples

<pre><code>r := router.New()    // 라우터 생성
r.AddListen(":3000") // listen 과정

wg := sync.WaitGroup{} // wg : 테스트 코드를 모두 수행할때 까지 wait시킬 WaitGroup
wg.Add(2)              // wg : 전송하고 받는 2가지 케이스를 wait

go func() {
    receiver, err := r.Accept(":3000", common.Coordinate{}) // 다른 라우터에서 dial 을 accept
    if err != nil {                                         // receiver를 받은 이후 에러가 발생 할 경우 패닉처리
        panic(err)
    }
    receiver.Send([]byte("test send")) // receiver에 test messgae 전송
    receiver.Flush()                   // flush
    wg.Done()                          // wg : 전송하는 케이스 완료
}()

go func() {
    otherRouter := router.New()                                              // 다른 라우터를 생성
    receiver, err := otherRouter.Dial("localhost:3000", common.Coordinate{}) // 로컬 호스트에 연결
    if err != nil {                                                          // receiver를 받은 이후 에러가 발생 할 경우 패닉처리
        panic(err)
    }
    bs, err := receiver.Recv() // message를 전송받을 대기중 맞 연결된 receiver에서 send/flush를 하면 []byte를 받음
    if err != nil {            // data를 받은 이후 에러가 발생 할 경우 패닉처리
        panic(err)
    }
    log.Println(string(bs)) // 전송받은 message를 출력
    wg.Done()               // wg : 받는 케이스 완료
}()

wg.Wait() // wg : 2가지 케이스가 완료 될 때까지 대기
</code></pre>

# Functions

<pre><code>type Router interface {
	AddListen(port string) error
	Dial(addr string, coordinate common.Coordinate) (Receiver, error)
	Accept(port string, coordinate common.Coordinate) (Receiver, error)
}
</code></pre>

<pre><code>type Receiver interface {
	Recv() ([]byte, error)
	Write(data []byte) (int, error)
	Send(data []byte) error
	Flush() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Close()
}
</code></pre>

## Router

### AddListen(port string) error

<pre><code>물리적 연결을 listen 하는 함수입니다.
다른 Router에서의 연결 요청을 대기 하고 있으며 요청이 발생 했을 경우 connection을 생성하여 읽기 대기 상태를 만듧니다.
멸티 체인에서 AddListen을 호출할 수 있으므로 중복된 포트요청을 인정합니다.
중복된 포트로 AddListen함수를 호출 하면 기존의 listen을 유지하며 AddListen함수에서는 추가적인 동작을 하지 않습니다.
</code></pre>

### Dial(addr string, coordinate common.Coordinate) (Receiver, error)

<pre><code>물리적 연결을 시도하는 함수 입니다.
AddListen 하고 있는 다른 Router에 물리연결 주소로 연결을 요청하여 물리적 연결이 수립되면 handshake과정을 거처 기제된 coordinate를 구분자로 listen하고 있는 router에 Accept요청을 합니다.
같은 router에 이미 물리연결이 되어있는 경우 handshake과정만 새로 거처 router에 Accept요청을 합니다.
</code></pre>

### Accept(port string, coordinate common.Coordinate) (Receiver, error)

<pre><code>AddListen에서 사용한 Port를 그대로 사용하며 할당받을 coordinate를 기제하여 
Dial요청이 들어오면 발생하면 handshake과정을 거처 Dial에 기제된 coordinate를 구분자로 사용하여 Accept요청자에게 Receiver를 리턴합니다.
Dial의 addr, coordinate와 1:1 쌍이 이룹니다.
</code></pre>

## Receiver

### Recv() ([]byte, error)

<pre><code>1:1쌍을 이루고 있는 연결에서 data를 기다립니다.
함수 호출 시점에서 wait하고 있다가 data가 전송된 시점에 []byte data를 리턴합니다.
</code></pre>

### Write(data []byte) (int, error)

<pre><code>데이터를 전송할때 사용합니다.
Send와 같은 일을 수행하지만 io.Writer interface에 맞추는 용도로 생성된 함수 입니다.
</code></pre>

#### examples

<pre><code>type WriteImplementStruct struct {
	checksum uint32
}

func (c *WriteImplementStruct) WriteTo(w io.Writer) (int64, error) {
	BNum := make([]byte, 4)
	binary.LittleEndian.PutUint32(BNum, checksum)
	if n, err := w.Write(BNum); err != nil {
		return int64(n), err
	} else if n != 4 {
		return int64(n), ErrInvalidLength
	} else {
		return 4, nil
	}
}

w := WriteImplementStruct{
	checksum : uint32(1)
}

w.WriteTo(receiver) // WriteImplementStruct에 기 할당된 Write 구조를 receiver에 전송
receiver.Flush()                       // flush
</code></pre>

### Send(data []byte) error

<pre><code>데이터를 전송할때 사용합니다.
Send함수를 호출하여 send buffer에 쌓는 역할을 수행합니다.
</code></pre>

### Flush() error

<pre><code>Send 혹은 Write함수를 호출하여 buffer에 쌓여있는 데이터를 flush하는 함수 입니다.
한 connection에서 여러 목표지점을 가지고 있는 multy data sending 환경에서 packet남비를 줄이기 위해 생성된 함수 입니다.

모든 Send와 Write함수는 데이터를 전송하기 위해 Flush함수를 호출해야합니다.
</code></pre>

#### examples

<pre><code>receiver.Send([]byte("test send")) // receiver에 test messgae 전송
receiver.Flush()                   // flush
</code></pre>

### LocalAddr() net.Addr

<pre><code>물리 연결된 connection의 Local Address를 리턴합니다.</code></pre>

### RemoteAddr() net.Addr

<pre><code>물리 연결된 connection의 Remote Address를 리턴합니다.
</code></pre>

### Close()

<pre><code>Receiver를 물리연결에서 제거 하며 물리 연결에 Receiver가 하나도 남지 않으면 물리 연결을 Close합니다.
</code></pre>

