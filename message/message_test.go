package message

import (
	"bytes"
	"reflect"
	"testing"

	"git.fleta.io/fleta/common/util"
	"git.fleta.io/fleta/framework/log"
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

			log.Debug(tt.args.messageType)
			tb := TypeToByte(tt.args.messageType)
			log.Debug(tb)

			buf := bytes.NewBuffer(tb)

			v, _, _ := util.ReadUint64(buf)

			mt := Type(v)
			log.Debug(mt)

			if got := DefineType(tt.args.t); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DefineType() = %v, want %v", got, tt.want)
			}
		})
	}
}
