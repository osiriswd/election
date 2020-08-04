package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kataras/iris"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/mvcc/mvccpb"
)

var (
	service    = flag.String("s", "service", "service name")
	host       = flag.String("h", "host1", "hostname")
	endpoint   = flag.String("e", "http://127.0.0.1:2379", "etcd host(:port)")
	listenport = flag.String("l", "8000", "listen port")
)

func init() {
	log.SetFlags(log.Lshortfile)
}

type Server struct {
	client         *clientv3.Client
	lease          clientv3.Lease
	leaseGrantResp *clientv3.LeaseGrantResponse
}

type Role struct {
	Service string `json:"service"`
	Host    string `json:"host"`
	Status  int    `json:"status"`
}

func NewServer(endpoints []string, NetworkAlive chan int) *Server {
	cfg := clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 2 * time.Second,
	}
	c, err := clientv3.New(cfg)
	if err != nil {
		log.Println(err)
		os.Exit(-1)
	}
	srv := &Server{}
	srv.client = c

	lease := clientv3.NewLease(srv.client)
	srv.lease = lease

	leaseGrantResp, err := lease.Grant(context.TODO(), 10)
	if err != nil {
		log.Println(err)
		os.Exit(-1)
	}
	srv.leaseGrantResp = leaseGrantResp

	leaseKeepAliveChan, err := lease.KeepAlive(context.TODO(), leaseGrantResp.ID)
	if err != nil {
		log.Println(err)
		os.Exit(-1)
	}

	go func() {
		var LastKAresponse *clientv3.LeaseKeepAliveResponse
		for {
			select {
			case LastKAresponse = <-leaseKeepAliveChan:
				if LastKAresponse == nil {
					NetworkAlive <- 0
				} else {
					NetworkAlive <- 1
				}
			}
		}
	}()

	return srv
}

func (srv *Server) register(key string, Primary chan int) (bool, error) {
	kv := clientv3.NewKV(srv.client)
	resp, err := kv.Txn(context.TODO()).
		If(clientv3.Compare(clientv3.Version(key), "=", 0)).
		Then(clientv3.OpPut(key, *host, clientv3.WithLease(srv.leaseGrantResp.ID))).
		Commit()
	if err != nil {
		return false, err
	}
	if resp.Succeeded {
		log.Printf("I am the primary of %s", *service)
		Primary <- 1
	} else {
		resp, err := srv.client.Get(context.TODO(), key)
		if err != nil {
			log.Println(err)
		}
		for _, kv := range resp.Kvs {
			log.Printf("%s is the primary of %s\n", kv.Value, *service)
			Primary <- 0
		}
	}
	return resp.Succeeded, nil
}

func (srv *Server) watch(key string) bool {
	wch := srv.client.Watch(context.TODO(), key)
	for wresp := range wch {
		for _, ev := range wresp.Events {
			switch ev.Type {
			case mvccpb.DELETE:
				return true
			}
		}
	}
	return false
}

func (srv *Server) Register(Primary chan int) {
	go func() {
		srv.register(*service, Primary)
		for {
			if srv.watch(*service) {
				srv.register(*service, Primary)
			}
		}
	}()
}

func main() {
	flag.Parse()
	Primary := make(chan int, 1)
	NetworkAlive := make(chan int, 1)
	NetworkAliveNow := 1
	go func() {
		defer close(Primary)
		defer close(NetworkAlive)
		log.Println("start...")
		endpoints := strings.Split(*endpoint, ",")
		srv := NewServer(endpoints, NetworkAlive)
		srv.Register(Primary)
		log.Println("end...")
		select {}
	}()

	go func() {
		for {
			select {
			case NetworkAliveNow = <-NetworkAlive:
			}
		}
	}()

	master := <-Primary
	app := iris.New()
	app.Get("/", func(ctx iris.Context) {
		if len(Primary) > 0 {
			master = <-Primary
		}
		status := master
		if NetworkAliveNow == 0 {
			status = -1
		}
		output := Role{Service: *service, Host: *host, Status: status}
		ctx.JSON(output)
	})
	app.Run(iris.Addr(fmt.Sprintf(":%s", *listenport)))
}
