package template

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"runtime"
	"strings"

	"golang.org/x/net/html"
)

type TemplateConfig struct {
	TemplatePath string
	LayoutPath   string
	WelcomeFile  string
}

func extractPage(n *html.Node, m map[string]interface{}) {
	for n != nil {
		if n.Type == html.ElementNode {
			var fletaID string
			for _, attr := range n.Attr {
				if attr.Key == "fletaid" {
					fletaID = strings.ToLower(attr.Val)
				}
			}

			if fletaID == "" {
				fletaID = strings.ToLower(n.Data)
				m[fletaID] = renderNode(n)
			} else {
				m[fletaID] = []byte(n.FirstChild.Data)
			}
		}
		n = n.NextSibling
	}
}

func getTag(doc *html.Node, tag string) (*html.Node, error) {
	var b *html.Node
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			b = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	if b != nil {
		return b, nil
	}
	return nil, errors.New("Missing <" + tag + "> in the node tree")
}

func renderNode(n *html.Node) []byte {
	var buf bytes.Buffer
	w := io.Writer(&buf)
	n = n.FirstChild
	for n != nil {
		html.Render(w, n)
		n = n.NextSibling
	}

	return []byte(html.UnescapeString(buf.String()))
}

type controll func(r *http.Request) (map[string][]byte, error)

func getMethod(v interface{}, methodName string) (reflect.Value, error) {
	methodName = strings.ToLower(methodName)

	fooType := reflect.TypeOf(v)
	if fooType == nil {
		return reflect.Value{}, errors.New("not found method")
	}
	for i := 0; i < fooType.NumMethod(); i++ {
		method := fooType.Method(i)
		if strings.ToLower(method.Name) == methodName {
			value := reflect.ValueOf(v)

			me := value.MethodByName(method.Name)
			if me.IsValid() {
				if isContollerMethod(me) {
					return me, nil
				}
			}

			return reflect.Value{}, errors.New("not found method")

		}
	}

	return reflect.Value{}, errors.New("not found method")
}

func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func callMethod(method reflect.Value, r *http.Request) (map[string]interface{}, error) {
	ret1 := method.
		Call(
			[]reflect.Value{
				reflect.ValueOf(r),
			},
		)
	e, _ := ret1[1].Interface().(error)
	m, is := ret1[0].Interface().(map[string]interface{})
	if !is {
		m, is := ret1[0].Interface().(map[string][]byte)
		if is {
			m2 := map[string]interface{}{}
			for k, v := range m {
				m2[k] = v
			}
			return m2, e
		}

		return nil, errors.New("is not controller method")
	}
	return m, e
}

func isContollerMethod(method reflect.Value) bool {
	methodType := method.Type()
	if methodType.NumIn() != 1 {
		return false
	}
	if methodType.NumOut() != 2 {
		return false
	}
	reqParam := methodType.In(0)
	if reqParam.String() != "*http.Request" {
		log.Println("reqParam.Name() : ", reqParam.String())
		return false
	}

	mapParam := methodType.Out(0)
	if mapParam.String() != "map[string][]uint8" && mapParam.String() != "map[string]interface {}" {
		log.Println("mapParam.Name() : ", mapParam.String())
		return false
	}
	errParam := methodType.Out(1)
	if errParam.String() != "error" {
		log.Println("errParam.Name() : ", errParam.String())
		return false
	}
	return true
}

// input parameter 정보 출력
func methodInMeta(method reflect.Value) {
	methodType := method.Type()
	fmt.Println(" input types:", methodType.NumIn())

	for j := 0; j < methodType.NumIn(); j++ {
		param := methodType.In(j)
		fmt.Printf("  in #%d : %s\n", j, param.Name())
	}
}

// output return 값 정보 출력
func methodOutMeta(method reflect.Value) {
	methodType := method.Type()
	fmt.Println(" output types:", methodType.NumOut())

	for j := 0; j < methodType.NumOut(); j++ {
		param := methodType.Out(j)
		fmt.Printf("  out #%d : %s\n", j, param.Name())
	}
}

func methodMetaData(method reflect.Value) {
	fmt.Println(method.Type())
	methodInMeta(method)
	methodOutMeta(method)
	fmt.Println("")
}
