package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-musthave-devops-trainer/internal/misc"
	"go-musthave-devops-trainer/internal/store"

	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
)

const (
	defaultAddress         = "localhost:8080"
	defaultShudownTimeout  = 5 * time.Second
	defaultRestoreFromFile = true
	defaultStoreFilename   = "/tmp/devops-metrics-db.json"
	defaultStoreInterval   = 5 * time.Minute
)

type config struct {
	address        string
	shudownTimeout time.Duration
	restoreOnStart bool
	storeInterval  time.Duration
	storeFile      string
	key            string
	databaseDSN    string
}

func main() {
	c := config{}

	flag.StringVar(&c.address, "a", defaultAddress, "address <<HOST:PORT>>")
	flag.DurationVar(&c.shudownTimeout, "s", defaultShudownTimeout, "timeout for shutdown")
	flag.BoolVar(&c.restoreOnStart, "r", defaultRestoreFromFile, "restore data from file on start")
	flag.DurationVar(&c.storeInterval, "i", defaultStoreInterval, "store interval for collected data")
	flag.StringVar(&c.storeFile, "f", defaultStoreFilename, "filename for store database")
	flag.StringVar(&c.key, "k", "", "key for sha256")
	flag.StringVar(&c.databaseDSN, "d", "", "Database DSN for PostgreSQL server")

	flag.Parse()

	c = config{
		address:        misc.GetEnvStr("ADDRESS", c.address),
		shudownTimeout: misc.GetEnvSeconds("SHUTDOWN_TIMEOUT", c.shudownTimeout),
		restoreOnStart: misc.GetEnvBool("RESTORE", c.restoreOnStart),
		storeInterval:  misc.GetEnvSeconds("STORE_INTERVAL", c.storeInterval),
		storeFile:      misc.GetEnvStr("STORE_FILE", c.storeFile),
		key:            misc.GetEnvStr("KEY", c.key),
		databaseDSN:    misc.GetEnvStr("DATABASE_DSN", c.databaseDSN),
	}

	if err := c.Run(context.Background()); err != nil {
		log.Fatalln("server:", err)
	}
	log.Println("server: gracefully stopped")
}

func (c *config) Run(ctx context.Context) error {
	log.Println("server: starting...")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	db, err := c.newStore(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	server := &serverStorage{
		db:  db,
		key: []byte(c.key),
	}

	srv := http.Server{
		Addr:    c.address,
		Handler: newRouter(server),
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	go func() {
		defer cancel()
		log.Println("server: listen monitor server on " + c.address)
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Println("HTTP server ListenAndServe:", err)
		}
	}()

	termSignal := make(chan os.Signal, 1)
	signal.Notify(termSignal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	select {
	case sig := <-termSignal:
		log.Println("server: shutting down... reason:", sig.String())
	case <-ctx.Done():
		log.Println("server: shutting down... reason:", ctx.Err().Error())
	}

	ctx, cancel = context.WithTimeout(ctx, c.shudownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	return nil
}

func (c *config) newStore(ctx context.Context) (storage store.Store, err error) {
	if c.databaseDSN != "" {
		rdb, err := newRDBStore(ctx, c.databaseDSN)
		if err != nil {
			return nil, fmt.Errorf("cannot create RDB store: %w", err)
		}
		if err := rdb.Bootstrap(ctx); err != nil {
			return nil, fmt.Errorf("cannot bootstrap RDB store: %w", err)
		}
		return rdb, nil
	}
	if c.storeFile != "" {
		db := store.NewFDB(ctx,
			store.WithRestoreOnStart(c.restoreOnStart),
			store.WithInterval(c.storeInterval),
			store.WithFile(c.storeFile))
		return db, nil
	}
	return nil, errors.New("unknown storage driver")
}

func newRDBStore(ctx context.Context, dsn string) (*store.RDB, error) {
	driverConfig := stdlib.DriverConfig{
		ConnConfig: pgx.ConnConfig{
			PreferSimpleProtocol: true,
		},
	}
	stdlib.RegisterDriverConfig(&driverConfig)

	conn, err := sql.Open("pgx", driverConfig.ConnectionString(dsn))
	if err != nil {
		return nil, fmt.Errorf("cannot create connection pool: %w", err)
	}

	if err = conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("cannot perform initial ping: %w", err)
	}

	return store.NewRDB(conn), nil
}
