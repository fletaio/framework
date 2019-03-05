package message

import (
	"bytes"
	"log"
	"reflect"
	"testing"

	"github.com/fletaio/common/util"
)

func TestDefineType(t *testing.T) {
	type args struct {
		t           string
		messageType Type
	}
	tests := []struct {
		name string
		args args
		want Type
	}{
		{
			name: "ping type",
			args: args{
				t:           "ping",
				messageType: DefineType("ping"),
			},
			want: DefineType("ping"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			tb := util.Uint64ToBytes(uint64(tt.args.messageType))

			buf := bytes.NewBuffer(tb)

			v, _, _ := util.ReadUint64(buf)

			mt := Type(v)
			log.Println(mt)

			if got := DefineType(tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DefineType() = %v, want %v", got, tt.want)
			}
		})
	}
}
