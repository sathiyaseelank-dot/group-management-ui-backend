package main

import (
	"context"
	"testing"
	"time"
)

type fakeGRPCStopper struct {
	gracefulStop func()
	stop         func()
}

func (f fakeGRPCStopper) GracefulStop() {
	if f.gracefulStop != nil {
		f.gracefulStop()
	}
}

func (f fakeGRPCStopper) Stop() {
	if f.stop != nil {
		f.stop()
	}
}

func TestShutdownGRPCServerGraceful(t *testing.T) {
	t.Helper()

	gracefulCalled := make(chan struct{}, 1)
	stopCalled := make(chan struct{}, 1)

	server := fakeGRPCStopper{
		gracefulStop: func() {
			gracefulCalled <- struct{}{}
		},
		stop: func() {
			stopCalled <- struct{}{}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	shutdownGRPCServer(ctx, server, false)

	select {
	case <-gracefulCalled:
	default:
		t.Fatal("expected GracefulStop to be called")
	}

	select {
	case <-stopCalled:
		t.Fatal("did not expect Stop to be called")
	default:
	}
}

func TestShutdownGRPCServerForcesStopAfterTimeout(t *testing.T) {
	t.Helper()

	gracefulStarted := make(chan struct{}, 1)
	unblockGraceful := make(chan struct{})
	stopCalled := make(chan struct{}, 1)

	server := fakeGRPCStopper{
		gracefulStop: func() {
			gracefulStarted <- struct{}{}
			<-unblockGraceful
		},
		stop: func() {
			stopCalled <- struct{}{}
			close(unblockGraceful)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	shutdownGRPCServer(ctx, server, false)

	select {
	case <-gracefulStarted:
	default:
		t.Fatal("expected GracefulStop to be started")
	}

	select {
	case <-stopCalled:
	default:
		t.Fatal("expected Stop to be called after timeout")
	}
}

func TestShutdownGRPCServerForceStop(t *testing.T) {
	t.Helper()

	gracefulCalled := make(chan struct{}, 1)
	stopCalled := make(chan struct{}, 1)

	server := fakeGRPCStopper{
		gracefulStop: func() {
			gracefulCalled <- struct{}{}
		},
		stop: func() {
			stopCalled <- struct{}{}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	shutdownGRPCServer(ctx, server, true)

	select {
	case <-stopCalled:
	default:
		t.Fatal("expected Stop to be called")
	}

	select {
	case <-gracefulCalled:
		t.Fatal("did not expect GracefulStop to be called")
	default:
	}
}
