package main

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

func Test_prepareCommand(t *testing.T) {
	type args struct {
		in     []string
		change string
	}
	tests := []struct {
		name    string
		args    args
		wantOut []string
	}{
		{
			"empty in",
			args{
				[]string{},
				"someReplace",
			},
			[]string{},
		},
		{
			"no replace",
			args{
				[]string{
					"someString",
				},
				"SomeReplace",
			},
			[]string{
				"someString",
			},
		},
		{
			"replace",
			args{
				[]string{
					"someString",
					ArgPlaceholder,
				},
				"someReplace",
			},
			[]string{
				"someString",
				"someReplace",
			},
		},
		{
			"replace substring",
			args{
				[]string{
					"someString" + ArgPlaceholder,
				},
				"SomeReplace",
			},
			[]string{
				"someStringSomeReplace",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotOut := prepareCommand(tt.args.in, tt.args.change); !reflect.DeepEqual(gotOut, tt.wantOut) {
				t.Errorf("prepareCommand() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

func TestNewCustomLimiterParallelLimit(t *testing.T) {
	l := NewCustomLimiter(MaxParallel(1), PerSecond(1000000))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var a, b bool
	var er error

	go func() {
		er = l.Get(ctx)
		a = true
	}()

	go func() {
		er = l.Get(ctx)
		b = true
	}()

	time.Sleep(time.Millisecond)

	if !(a || b) {
		t.Error("ParallelLimited scope not executed")
	}

	if a && b {
		t.Error("ParallelLimited scope executed twice")
	}

	l.Put()

	time.Sleep(time.Millisecond)

	if !(a && b) {
		t.Error("ParallelLimited scope not executed twice after Put")
	}

	if er != nil {
		t.Error("ParallelLimited returns error in happy story")
	}
}

func TestNewCustomLimiterContextCanceling(t *testing.T) {
	l := NewCustomLimiter(MaxParallel(0), PerSecond(0))
	ctx, cancel := context.WithCancel(context.Background())
	var a bool

	var er error
	go func() {
		er = l.Get(ctx)
		a = true
	}()

	time.Sleep(time.Millisecond)

	if er != nil {
		t.Error("ParallelLimited not blocked")
	}

	cancel()

	time.Sleep(time.Millisecond)

	if !a {
		t.Error("ParallelLimited canceling context did not unblock limiter")
	}

	if er != context.Canceled {
		t.Error("ParallelLimited did not return context.Canceled")
	}
}

func TestNewCustomLimiterRateLimitPerSecond(t *testing.T) {
	l := NewCustomLimiter(MaxParallel(10), PerSecond(5))
	ctx, cancel := context.WithCancel(context.Background())

	cnt := counter{}

	for i := 0; i < 10; i++ {
		go func() {
			for {
				if l.Get(ctx) != nil {
					break
				}
				cnt.Add()
				time.Sleep(time.Millisecond)
				l.Put()
			}
		}()
	}
	time.Sleep(time.Second)
	cancel()
	if cnt.Count() != 5 {
		t.Errorf("ParallelLimited scope executed %v times, want %v", cnt.Cnt, 5)
	}
}

type counter struct {
	Cnt int
	sync.Mutex
}

func (c *counter) Add() {
	c.Lock()
	c.Cnt++
	c.Unlock()
}

func (c *counter) Count() int {
	c.Lock()
	defer c.Unlock()
	return c.Cnt
}
