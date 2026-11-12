package main

import (
	"context"
	"log"
	"net"
	"os"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/zm1464688361-svg/pa/internal/biz"
	"github.com/zm1464688361-svg/pa/internal/clock"
	"github.com/zm1464688361-svg/pa/internal/config"
	"github.com/zm1464688361-svg/pa/internal/repo"
	petshop "github.com/zm1464688361-svg/pa/kitex_gen/petshop/appointmentservice"
)

func main() {
	dsn := config.LoadDSN()
	db, err := repo.Open(dsn, true)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := repo.EnsureClockRow(context.Background(), db); err != nil {
		log.Fatalf("ensure clock row: %v", err)
	}
	clk := clock.New(&clock.DBStore{DB: db})
	impl := &AppointmentServiceImpl{B: &biz.Biz{DB: db, Clock: clk}}

	addr, _ := net.ResolveTCPAddr("tcp", getenv("RPC_ADDR", ":9001"))
	svr := petshop.NewServer(impl,
		server.WithServiceAddr(addr),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "appointmentservice"}),
	)
	if err := svr.Run(); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
