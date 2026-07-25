package scenarios

import (
	"bunyan"
	"context"
	"fmt"
	"reflect"
	"time"
)

func Simple() error {

	testDuration := 2 * time.Second
	bg := context.Background()

	testPrintln("Simple duration: ", testDuration)

	// blocking outer context -- bunyan.NewManager runs in the background
	holdOpen, holdOpenCancel := context.WithTimeout(bg, testDuration)
	defer holdOpenCancel()

	startTime := time.Now()
	defer func(start time.Time) {
		fmt.Printf("example simple held open for: %v.  happy hacking!\n", time.Since(start))
	}(startTime)

	managerID := bunyan.ManagerID("simple_manager")
	manager := bunyan.NewManager(bg, managerID, nil)

	processCtx, processCtxCancel := context.WithTimeout(bg, 3*time.Second)
	defer processCtxCancel()

	zoneCtx, zoneCtxCancel := context.WithTimeout(processCtx, 2*time.Second)
	defer zoneCtxCancel()

	zoneID := bunyan.ZoneID("simple_zone")
	zoneSimple, err := manager.NewZone(zoneCtx, zoneID)
	if err != nil {
		panic(err)
	}
	// zone implicitly closed

	chainContext, chainContextCancel := context.WithTimeout(processCtx, 1*time.Second)
	chainID := bunyan.ChainID("simple_chain")
	// simple chain tracking.  this does not implicitly start.
	chainSimple := zoneSimple.NewChain(chainContext, chainID)

	// chain implicitly closed by zone
	_ = chainContextCancel

	simpleChainComment := "we're just testing a simple chain"
	chainSimple.AddComment(simpleChainComment)

	// having an empty category is also permissible
	spanID := bunyan.SpanID("simple_span_id")
	spanSimple := chainSimple.NewSpan("simple_category_for_span", spanID, bunyan.SpanFields{
		ParentSpanID: "example_parent_span_id",
		Comment:      nil,
	})

	spanSimple.Start()
	simpleComment := "we're just testing a simple span"
	spanSimple.AddComment(simpleComment)

	spanSimple.End()

	/*
		call to report clears out fields?
		start/end no longer tracking?
	*/

	// without holding open, we might not process the span end in time
	// an improved test without a race condition could use get functionality or
	// polling for data to be available on a copy
	<-holdOpen.Done()

	// note that a different path is taken in Report() when the chain is closed - which marks no updates are possible
	report, err := chainSimple.Report()
	if err != nil {
		return err
	}

	// len just in case somehow we end up comparing empty strings. the concern is mostly in lost values.
	if report.ID != chainID || len(report.ID) == 0 {
		return fmt.Errorf("simple test: report id mismatch.  want: %s; got: %s", chainID, report.ID)
	}

	if !reflect.DeepEqual(report.Categories.Comment, []string{simpleChainComment}) {
		return fmt.Errorf("simple test: report comment mismatch.  want: %s; got: %s", []string{simpleChainComment}, report.Categories.Comment)
	}

	fmt.Println(report)

	// todo: assigned while debugging
	_ = spanSimple

	return nil
}
