package research

import (
	"fmt"
	"testing"
)

func TestBlockSegmentList(t *testing.T) {
	flags := []string{
		"1", "1_001", "1-2", "1_001-2_000",
		"1_000-2_000k", "1-2M",
		"1,1-2M,1_001", "1_000-2_000k,1-2,1_001-2_000",
	}
	brs := []BlockSegmentList{
		{
			NewBlockSegment(1, 1),
		},
		{
			NewBlockSegment(1001, 1001),
		},
		{
			NewBlockSegment(1, 2),
		},
		{
			NewBlockSegment(1001, 2000),
		},
		{
			NewBlockSegment(1000001, 2000000),
		},
		{
			NewBlockSegment(1000001, 2000000),
		},
		{
			NewBlockSegment(1, 1),
			NewBlockSegment(1000001, 2000000),
			NewBlockSegment(1001, 1001),
		},
		{
			NewBlockSegment(1000001, 2000000),
			NewBlockSegment(1, 2),
			NewBlockSegment(1001, 2000),
		},
	}
	for i, flag := range flags {
		br, err := ParseBlockSegmentList(flag)
		if err != nil {
			panic(err)
		}
		if len(brs[i]) != len(br) {
			panic(fmt.Errorf("number of parse block ranges are different"))
		}
		for j, bs1 := range brs[i] {
			bs2 := br[j]
			if bs1.First != bs2.First || bs1.Last != bs2.Last {
				panic(fmt.Errorf("block segment is not same: %v %v", bs1, bs2))
			}
		}
	}
}

func TestBlockSegmentListBad(t *testing.T) {
	flags := []string{
		"", ",", "1x", "1-", "-1",
		"1k", "2M",
		"1,2M", "1,1-", "1-,1", "1,-1", "-1,1",
	}
	for _, flag := range flags {
		_, err := ParseBlockSegmentList(flag)
		if err == nil {
			panic(fmt.Errorf("error is not raised for bad flags"))
		}
	}
}
