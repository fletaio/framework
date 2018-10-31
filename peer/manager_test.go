package peer

import (
	"reflect"
	"testing"

	"git.fleta.io/fleta/common"
	"git.fleta.io/fleta/message"
)

func TestNewManager(t *testing.T) {
	type args struct {
		ChainCoord *common.Coordinate
		mh         *message.Handler
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test",
			args: args{
				ChainCoord: &common.Coordinate{},
				mh:         message.NewHandler(),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewManager(tt.args.ChainCoord, tt.args.mh); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewManager() = %v, want %v", got, tt.want)
			}
		})
	}
}
