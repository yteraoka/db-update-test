package main

import (
	"flag"
	"fmt"
	"os"
	"log"
	"math/rand"
	"sync"
	"time"

	_ "github.com/lib/pq"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/google/uuid"
	"github.com/aybabtme/uniplot/histogram"
)

type Sequence struct {
	Id      string `db:"id"`
	Counter int64  `db:"counter"`
}

var verbose bool
var warnThresholdMs int64
var dbServer string
var db *sqlx.DB

// DNS
// user=foo password=secret dbname=bar host=127.0.0.1 sslmode=disable

func initTable(records int) {
	log.Println("starting table initialize")
	db.MustExec("DROP TABLE IF EXISTS sequences")
	db.MustExec("CREATE TABLE sequences (id varchar(36), counter bigint, PRIMARY KEY (id))")
	for i := 0; i < records; i++ {
		if dbServer == "mysql" {
			db.MustExec("INSERT INTO sequences (id, counter) VALUES (?, 0)", uuid.NewString())
		} else {
			db.MustExec("INSERT INTO sequences (id, counter) VALUES ($1, 0)", uuid.NewString())
		}
	}
	log.Println("table initialized")
}

func main() {
	var workers, updateCount, initRecords, maxConns int
	var doInit bool

	flag.BoolVar(&verbose, "verbose", false, "enable verbose output")
	flag.IntVar(&updateCount, "update-count", 1, "update count each record")
	flag.IntVar(&workers, "workers", 1, "workers")
	flag.IntVar(&initRecords, "init-records", 1000, "Number of records to insert in initialize")
	flag.IntVar(&maxConns, "max-connections", 10, "Max db connections")
	flag.BoolVar(&doInit, "init", false, "do truncate and insert records")
	flag.Int64Var(&warnThresholdMs, "warn-threshold", 50, "duration output threshold in millisecond")
	flag.StringVar(&dbServer, "db-server", "mysql", "mysql or postgres")
	flag.Parse()

	var err error

	rand.Seed(time.Now().UnixNano())

	dsn := os.Getenv("DSN")
	if dsn == "" {
		log.Fatal("DSN environment variable required. [username[:password]@][protocol[(address)]]/dbname[?param1=value1&...&paramN=valueN]")
	}

	db, err = sqlx.Connect(dbServer, os.Getenv("DSN"))
	if err != nil {
		log.Fatal(err)
	}

	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(10)

	if doInit {
		initTable(initRecords)
	}

	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	jobChannel := make(chan string, 2)
	responseChannel := make(chan int64, 1)

	wgResp := sync.WaitGroup{}
	go responseChecker(&wgResp, responseChannel)
	wgResp.Add(1)

	seqs := []Sequence{}

	err = db.Select(&seqs, "SELECT id, counter FROM sequences ORDER BY id")

	wgWorker := sync.WaitGroup{}
	for i := 0; i < workers; i++ {
		go IncrWorker(&wgWorker, jobChannel, responseChannel)
		wgWorker.Add(1)
	}

	for i := 0; i < updateCount; i++ {
		s := seqs[rand.Intn(len(seqs))]
		jobChannel <- s.Id
	}

	close(jobChannel)

	wgWorker.Wait()

	if verbose {
		seqs = []Sequence{}
		err = db.Select(&seqs, "SELECT id, counter FROM sequences ORDER BY id")
		for _, s := range seqs {
			fmt.Printf("AFTER %s = %d\n", s.Id, s.Counter)
		}
	}

	close(responseChannel)

	wgResp.Wait()
}

func IncrWorker(wg *sync.WaitGroup, ch <-chan string, respCh chan int64) {
	defer wg.Done()
	for {
		id, more :=  <-ch
		if more == false {
			break
		}

		tx, err := db.Beginx()
		if err != nil {
			log.Fatalln(err)
		}
		seq := Sequence{}

		t1 := time.Now()

		if dbServer == "mysql" {
			err = tx.Get(&seq, "SELECT id, counter FROM sequences WHERE id = ? FOR UPDATE", id)
		} else {
			err = tx.Get(&seq, "SELECT id, counter FROM sequences WHERE id = $1 FOR UPDATE", id)
		}
		if err != nil {
			log.Println(err)
			tx.Rollback()
			continue
		}
		if dbServer == "mysql" {
			_, err = tx.Exec("UPDATE sequences SET counter = ? + 1 WHERE id = ?", seq.Counter, id)
		} else {
			_, err = tx.Exec("UPDATE sequences SET counter = $1 + 1 WHERE id = $2", seq.Counter, id)
		}
		if err != nil {
			log.Println(err)
			tx.Rollback()
			continue
		}
		err = tx.Commit()
		if err != nil {
			log.Println(err)
			tx.Rollback()
			continue
		}

		t2 := time.Now()
		diff := t2.Sub(t1).Milliseconds()
		if diff > warnThresholdMs {
			log.Printf("update %s took %d ms\n", id, diff)
		}
		respCh <- diff
	}
}

func responseChecker(wg *sync.WaitGroup, ch <- chan int64) {
	defer wg.Done()
	var min, max, sum int64
	data := make([]float64, 0, 1000)
	for {
		msec, more := <-ch
		if more == false {
			break
		}
		data = append(data, float64(msec))
		if msec > max {
			max = msec
		}
		if (min == 0 || min > msec) {
			min = msec
		}
		sum += msec
	}

	hist := histogram.Hist(10, data)
	fmt.Printf("Count: %d\n", hist.Count)
	fmt.Printf("Min: %d ms\n", min)
	fmt.Printf("Max: %d ms\n", max)
	fmt.Printf("Ave: %v ms\n", sum / int64(hist.Count))
	maxWidth := 5
	fmt.Println("[histogram]")
	err := histogram.Fprint(os.Stdout, hist, histogram.Linear(maxWidth))
	if err != nil {
		log.Fatalln(err)
	}
}
