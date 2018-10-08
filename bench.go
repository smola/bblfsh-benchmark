package main

import (
	"context"
	"sort"
	"sync"
	"time"

	"gopkg.in/bblfsh/client-go.v3"
	"gopkg.in/src-d/go-log.v1"
)

type benchmarkOpts struct {
	Bblfsh   string
	Workers  int
	Step     time.Duration
	Content  string
	Language string
}

type request struct {
	Content  string
	Language string
}

type result struct {
	New      bool
	WorkerID int
	Err      error
	Start    time.Time
	End      time.Time
}

func benchmark(ctx context.Context, opts benchmarkOpts) error {
	requests := make(chan *request)
	results := make(chan *result)
	stopProduce := make(chan struct{})
	stopResults := make(chan struct{})

	req := &request{
		Content:  opts.Content,
		Language: opts.Language,
	}
	go produceRequests(stopProduce, req, requests)
	go processResults(stopResults, results)

	wg := &sync.WaitGroup{}
	for w := 1; w <= opts.Workers; w++ {
		wg.Add(1)
		log.Debugf("Instantiating client %d", w)
		client, err := bblfsh.NewClient(opts.Bblfsh)
		if err != nil {
			log.Errorf(err, "Instantiating client %d", w)
			return err
		}
		log.Debugf("Instantiated client %d", w)

		go func() {
			results <- &result{New: true, WorkerID: w, Start: time.Now()}
			work(w, client, ctx.Done(), requests, results)
			log.Debugf("Worker %d finished", w)
			wg.Done()
		}()

		<-time.After(opts.Step)
	}

	log.Infof("Stopping")
	close(stopProduce)
	log.Infof("Waiting for all workers to finish")
	wg.Wait()
	close(results)
	<-stopResults

	return nil
}

func produceRequests(stop <-chan struct{}, req *request, requests chan<- *request) {
	for {
		select {
		case <-stop:
			close(requests)
			return
		default:
		}

		requests <- req
	}
}

func processResults(stop chan<- struct{}, results <-chan *result) {
	var (
		resultSlice     []*result
		resultIncreases []*time.Time
	)

	for {
		res, ok := <-results
		if !ok {
			break
		}

		if res.New {
			resultIncreases = append(resultIncreases, &res.Start)
			continue

		}

		resultSlice = append(resultSlice, res)
	}

	sort.Slice(resultSlice, func(i, j int) bool {
		return resultSlice[i].Start.Before(resultSlice[j].Start)
	})

	for i := 0; i < len(resultIncreases); i++ {
		start := resultIncreases[i]
		var end *time.Time
		if i+1 < len(resultIncreases) {
			end = resultIncreases[i+1]
		}

		var (
			total    int
			success  int
			errors   int
			min, max time.Duration
			lastEnd  time.Time
		)

		for _, r := range resultSlice {
			if r.Start.Before(*start) {
				continue
			}

			if end != nil && r.Start.After(*end) {
				continue
			}

			total++
			if r.Err == nil {
				success++
			} else {
				errors++
			}

			t := r.End.Sub(r.Start)
			if min == 0 || min > t {
				min = t
			}

			if max == 0 || max < t {
				max = t
			}

			if lastEnd.IsZero() || r.End.After(lastEnd) {
				lastEnd = r.End
			}
		}

		if end == nil {
			end = &lastEnd
		}

		totalDuration := (*end).Sub(*start)
		rps := float64(success) / totalDuration.Seconds()
		log.Infof("Result: workers=%d reqs=%d errors=%d min=%s max=%s rps=%f",
			i+1, total, errors, min, max, rps)
	}

	close(stop)
}

func work(id int, client *bblfsh.Client, done <-chan struct{},
	requests <-chan *request, results chan<- *result) {

	for {
		select {
		case <-done:
			return
		case req, ok := <-requests:
			if !ok {
				return
			}

			results <- do(id, client, req)
		}
	}
}

func do(id int, client *bblfsh.Client, req *request) *result {
	res := &result{WorkerID: id}
	res.Start = time.Now()
	_, res.Err = client.NewParseRequest().Content(req.Content).Language(req.Language).Do()
	res.End = time.Now()
	return res
}
