package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"
)

const ArgPlaceholder = "{}"

var rate = flag.Float64("rate", 1, "exec per second limit")
var inflight = flag.Int("inflight", 1, "exec max parallel")
var command []string

func main() {
	flag.Parse()
	command = flag.Args()
	if len(command) == 0 {
		log.Fatal("no command")
	}

	ctx := makeContext()
	scanner := bufio.NewScanner(os.Stdin)
	l := NewCustomLimiter(MaxParallel(*inflight), PerSecond(*rate))
	for scanner.Scan() {
		if err := l.Get(ctx); err != nil {
			break
		}
		argToFill := scanner.Text()
		go func() {
			defer l.Put()
			cmdLine := prepareCommand(command, argToFill)
			cmd := exec.CommandContext(ctx, cmdLine[0], cmdLine[1:]...)
			out, err := cmd.Output()
			if err != nil {
				log.Println(err.Error())
			}
			log.Print(string(out))
		}()
	}
	l.Wait()
	log.Println("fini")
}

func makeContext() context.Context {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		cancel()
		signal.Stop(c)
	}()
	return ctx
}

func prepareCommand(in []string, change string) (out []string) {
	out = make([]string, len(in))
	for i, s := range in {
		out[i] = strings.Replace(s, ArgPlaceholder, change, -1)
	}
	return out
}

type CustomLimiter struct {
	inflightCh chan struct{}
	rateCh     chan struct{} // time/rate is not part of stdlib, and time.Ticker does not fit here
	sync.WaitGroup
}

type PerSecond float64
type MaxParallel int

func NewCustomLimiter(inflight MaxParallel, ratePerSecond PerSecond) CustomLimiter {
	l := CustomLimiter{
		make(chan struct{}, inflight),
		make(chan struct{}),
		sync.WaitGroup{},
	}
	go func() {
		for {
			l.rateCh <- struct{}{}
			time.Sleep(time.Duration(float64(time.Second) / float64(ratePerSecond)))
		}
	}()
	return l
}

func (l *CustomLimiter) Get(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case l.inflightCh <- struct{}{}: //get inflight limiter lock
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.rateCh: //then get ratePerSecond limiter lock
	}

	l.Add(1)
	return nil
}

func (l *CustomLimiter) Put() {
	<-l.inflightCh
	l.Done()
}
