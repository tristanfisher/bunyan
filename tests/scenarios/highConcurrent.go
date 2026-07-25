package scenarios

import (
	"context"
	"fmt"
	"math/rand"
	mathRand "math/rand/v2"
	"strings"
	"time"
)

var charset = []rune("abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
	"0123456789" +
	"абвгдеёжзийклмнопрстуфхцчшщъыьэюя" +
	"АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ")

var charsetLen = len(charset)

// const for test to reduce size of function call
const minLengthRandomChars = 4
const maxLengthRandomChars = 50

func randomNumberInRange(low int, high int) int {
	return low + mathRand.IntN(high-low)
}

func getRandomID() string {

	// consider mathRand.NewPCG() (Permuted Congruential Generator)
	// if refactoring for higher performance by putting random into a single thread
	//
	// conceivably, we could generate random selection lengths, then pick one at random.
	// this would allow for using a PCG in a single thread, bypassing constant calls to rand.IntN

	// returns random length from [0, n)
	// shift to min, n (subtract minLengthRandomChars so we don't go out of range)
	randomCharLength := randomNumberInRange(minLengthRandomChars, maxLengthRandomChars)
	var sb strings.Builder
	sb.Grow(randomCharLength)

	// for our random length, grab a random char from charset
	//for i := 0; i < randomCharLength; i++ {
	//	sb.WriteRune(charset[rand.Intn(charsetLen)])
	//}

	for range randomCharLength {
		sb.WriteRune(charset[rand.Intn(charsetLen)])
	}

	return sb.String()
}

type ZoneConfig struct {
	ID     string
	Chains []ChainConfig
}

type ChainConfig struct {
	ID    string
	Spans []SpanConfig
}

type SpanConfig struct {
	ID string
	// when get parent ID is available, add in a % chance of
	// associating a given span with another via parentage
	Comment              []string
	PercentChangeComment float64
}

type HCConfig struct {
	ManagerID string
	Zones     []ZoneConfig
}

// HighConcurrency aka fuzztest
func HighConcurrency(testDuration time.Duration) {

	testPrintln("High concurrency test duration: ", testDuration)

	startTime := time.Now()
	defer func(start time.Time) {
		fmt.Printf("example morning held open for: %v.  happy hacking!\n", time.Since(start))
	}(startTime)

	// test prep happens outside of test duration

	// [5 to 15) managers in this process
	managerCount := 5 + mathRand.IntN(15)
	managerConfigs := make([]HCConfig, managerCount)

	// pre-calculate counts, IDs, arguments. this is not allocation of spans/zones/etc.
	for managerIdx := 0; managerIdx < managerCount; managerIdx++ {

		zoneCount := randomNumberInRange(5, 10)
		managerConfigs[managerIdx] = HCConfig{
			ManagerID: "manager_" + getRandomID(),
			Zones:     make([]ZoneConfig, zoneCount),
		}

		// name and make chains for ech zone
		for i := 0; i < zoneCount; i++ {
			chainCount := randomNumberInRange(0, 100)
			managerConfigs[managerIdx].Zones[i] = ZoneConfig{
				ID:     "zone_" + getRandomID(),
				Chains: make([]ChainConfig, chainCount),
			}

			// then dive into spans for those chains
			for chainIdx := 0; chainIdx < chainCount; chainIdx++ {
				spanCount := randomNumberInRange(0, 250)
				managerConfigs[managerIdx].Zones[i].Chains[chainIdx] = ChainConfig{
					ID:    "chain_" + getRandomID(),
					Spans: make([]SpanConfig, spanCount),
				}

				for spanIdx := 0; spanIdx < spanCount; spanIdx++ {
					managerConfigs[managerIdx].Zones[i].Chains[chainIdx].Spans[spanIdx] = SpanConfig{
						ID: "span_" + getRandomID(),
					}
				}

			}
		}
	}

	_ = managerConfigs

	bg := context.Background()
	holdOpen, holdOpenCancel := context.WithTimeout(bg, testDuration)
	defer holdOpenCancel()
	go func(c context.Context) {

	}(holdOpen)
	<-holdOpen.Done()
}
