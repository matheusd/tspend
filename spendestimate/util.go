package main

import (
	"fmt"
	"time"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/dcrutil/v4"
)

// formatDuration formats a duration with a "day" section whenever the duration
// is longer than 24 hours.
func formatDuration(d time.Duration) string {
	day := 24 * time.Hour
	if d > day {
		days := d / day
		hours := (d - days*day) / time.Hour
		return fmt.Sprintf("%dd%dh", days, hours)
	}

	return d.Truncate(time.Minute).String()
}

// sumTbases sums treasury bases inside a given block window ending at
// endHeight, taking into account subsidy reductions that happen along the way.
//
// This is inclusive of both the endHeight block and the starting block
// (endHeight - blocks).
func sumTbases(endHeight, blocks, subReductionInterval int64, subCache *standalone.SubsidyCache) dcrutil.Amount {
	var res int64
	startHeight := endHeight - blocks + 1
	height := startHeight
	for height <= endHeight {
		blocksToAdd := subReductionInterval
		flags := []byte{0x20, 0x20}
		if height%subReductionInterval != 0 {
			blocksToAdd = subReductionInterval - (height % subReductionInterval)
			flags[0] = '%'
		}
		if height+blocksToAdd > endHeight {
			blocksToAdd = endHeight - height + 1
			flags[1] = 'f'
		}
		tbase := subCache.CalcTreasurySubsidy(height, 5, true)
		res += tbase * blocksToAdd
		// println("XXXXXXX %s add %s * %d @ %d - sum %s", flags,
		//			dcrutil.Amount(tbase), blocksToAdd, height, dcrutil.Amount(res))
		height += blocksToAdd
	}

	return dcrutil.Amount(res)
}

func plural(i int64, one, many string) string {
	if i == 1 {
		return one
	}
	return many
}
