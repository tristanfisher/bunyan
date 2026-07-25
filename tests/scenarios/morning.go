package scenarios

import (
	"bunyan"
	"context"
	"fmt"
	"log"
	"time"
)

// time.sleep in example to simulate a timeline / "work"

func Morning() error {

	testDuration := 10 * time.Second
	bg := context.Background()

	testPrintln("Morning duration: ", testDuration)

	// blocking outer context -- bunyan.NewManager runs in the background
	holdOpen, holdOpenCancel := context.WithTimeout(bg, testDuration)
	defer holdOpenCancel()

	// print execution time to show the manager is held open in the background
	startTime := time.Now()
	defer func(start time.Time) {
		fmt.Printf("example morning held open for: %v.  happy hacking!\n", time.Since(start))
	}(startTime)

	// bunyan example:

	managerID := bunyan.ManagerID("example_manager")
	manager := bunyan.NewManager(bg, managerID, nil)

	processCtx, processCtxCancel := context.WithTimeout(bg, 5*time.Second)
	defer processCtxCancel()

	// a zone can have a separate context from the base manager
	// this allows zones to terminate, while preserving the "base" manager instance
	zoneCtx, zoneCtxCancel := context.WithTimeout(processCtx, 3*time.Second)
	defer zoneCtxCancel()

	zoneID := bunyan.ZoneID("example_zone")
	zoneMorning, err := manager.NewZone(zoneCtx, zoneID)
	if err != nil {
		panic(err)
	}

	chainContext, chainContextCancel := context.WithTimeout(processCtx, 2*time.Second)
	defer chainContextCancel()

	worldChain := zoneMorning.NewChain(chainContext, "world_chain")
	_ = worldChain

	// repeat category to test that existing span categories work as intended
	sunCategoryShared := "sun"

	sunCycle := func(c context.Context) {
		// note this will get implicitly closed
		sunRiseSpan := worldChain.NewSpan(sunCategoryShared, "sun up", bunyan.SpanFields{
			ParentSpanID: "sun",
			Comment:      []string{"sun popped over the horizon"},
		})

		// TODO: put a time.Sleep() here of 10 seconds to test
		// 		fatal error: all goroutines are asleep - deadlock!
		// the issue is likely exceeding `testDuration`
		//time.Sleep(10 * time.Second)
		time.Sleep(10 * time.Millisecond)

		sunRiseSpan.Start()

		sunSpan := worldChain.NewSpan(sunCategoryShared, "sun in the sky", bunyan.SpanFields{
			ParentSpanID: "sun",
		})
		sunSpan.Start()
		sunRiseSpan.End()
		// note this will get implicitly closed by the Chain closing
		//sunSpan.End()
	}
	// context for process cleanliness
	sunCycle(chainContext)
	// todo testadd tests for closing context:
	// 1. close explicitly
	// 2. close by context timeout
	// 3. close by zone context
	// 4. close by chain context (chainContextCancel)
	//    notably: if chain context is cancelled before a copy operation is attempted,
	//             we will throw - fatal error: all goroutines are asleep - deadlock!
	//
	// time.Sleep(time.Second) beyond chainContext in handleLifetime will exercise closure before initialization
	//
	// a chain can have a separate context from the base manager or zone
	// this allows zones to terminate, while preserving the "base" manager instance or zone
	chainID := bunyan.ChainID("example_chain using its own context")
	morningRitualChain := zoneMorning.NewChain(chainContext, chainID)
	//
	//// todo: test timingCopy in a goroutine
	//// prove we can make a simple copy and operate off of it
	morningRitualChainCopy, err := morningRitualChain.TimingCopy()
	if err != nil {
		log.Fatal(err)
	}

	//// note report before the span has anything put on it!  this is safe to call at any point
	//// even on a copy

	// this is what fails apparently
	morningRitualReport, err := morningRitualChainCopy.Report()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("morning ritual report => ", morningRitualReport)

	//
	//snoozeSpan := morningRitualChain.NewSpan("sleep", "snooze", bunyan.SpanFields{
	//	ParentSpanID: "",
	//	Comment:      []string{"hit snooze button"},
	//})
	//snoozeSpan.Start()
	//// take a little nap
	//time.Sleep(100 * time.Millisecond)
	//snoozeSpan.End()
	//
	//coffeeSpan := morningRitualChain.NewSpan("wakeup_tasks", "machine_on", bunyan.SpanFields{
	//	ParentSpanID: "coffee", // note use as parent, not previously stated
	//})
	//
	//time.Sleep(10 * time.Millisecond)
	//// coffee machine still running!
	//showerSpan := morningRitualChain.NewSpan("wakeup_tasks", "shower", bunyan.SpanFields{})
	//time.Sleep(50 * time.Millisecond)
	//showerSpan.End()
	//
	//dressSpan := morningRitualChain.NewSpan("wakeup_tasks", "get dressed", bunyan.SpanFields{})
	//dressSpan.Start()
	//
	//pickClothesSpan := morningRitualChain.NewSpan("wakeup_tasks", "pick clothes", bunyan.SpanFields{
	//	ParentSpanID: "get dressed",
	//})
	//pickClothesSpan.Start()
	//time.Sleep(2 * time.Millisecond)
	//pickClothesSpan.End()
	//
	//// coffee's reaady!
	//coffeeSpan.End()
	//
	//putClothesOnSpan := morningRitualChain.NewSpan("wakeup_tasks", "put clothes on", bunyan.SpanFields{
	//	ParentSpanID: "get dressed",
	//})
	//putClothesOnSpan.Start()
	//time.Sleep(2 * time.Millisecond)
	//putClothesOnSpan.End()
	//
	//dressSpan.End()
	//
	//morningRitualChain.Close()
	//
	//// zone close will close a chain, which cascades "downstream" to Chain, etc
	//err = zoneMorning.Close()
	//if err != nil {
	//	return err
	//}
	//
	//testPrintln(zoneMorning.Report())

	<-holdOpen.Done()

	return nil
}
