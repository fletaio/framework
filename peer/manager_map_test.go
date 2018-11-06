package peer

import (
	"log"
	"reflect"
	"testing"

	"git.fleta.io/fleta/framework/peer/peermessage"
)

func TestNewNodeStore(t *testing.T) {
	type args struct {
		TempMockID string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test",
			args: args{
				TempMockID: "TempMockID",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := NewNodeStore(tt.args.TempMockID)

			var ci peermessage.ConnectInfo
			ci.Address = "test3"
			ci.PingTime = 1
			ci.ScoreBoard = &peermessage.ScoreBoardMap{}

			got.Store(ci.Address, ci)

			ci2, has := got.Load("test")
			result := false
			if has {
				result = ci2.PingTime == ci.PingTime
			}

			log.Println("got.Len() : ", got.Len())

			if !reflect.DeepEqual(result, tt.want) {
				t.Errorf("result = %v, want %v", result, tt.want)
			}
		})
	}
}
