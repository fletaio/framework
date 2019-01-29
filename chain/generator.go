package chain

type Generator interface {
	Generate() (*Data, interface{}, error)
}
