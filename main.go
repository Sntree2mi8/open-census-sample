package main

import (
	"context"
	"contrib.go.opencensus.io/exporter/jaeger"
	"contrib.go.opencensus.io/integrations/ocsql"
	"database/sql"
	"errors"
	"github.com/Sntree2mi8/open-census-sample/pkg/ocgorm"
	"github.com/go-sql-driver/mysql"
	legacygorm "github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"go.opencensus.io/trace"
	gmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
)

var mysqlconf = mysql.Config{
	User:   "root",
	Net:    "tcp",
	Addr:   "localhost:13306",
	DBName: "ocsample",
}

func main() {
	agentEndpointURI := "localhost:6831"
	collectorEndpointURI := "http://localhost:14268/api/traces"

	je, err := jaeger.NewExporter(jaeger.Options{
		AgentEndpoint:     agentEndpointURI,
		CollectorEndpoint: collectorEndpointURI,
		Process: jaeger.Process{
			ServiceName: "open-census-practice",
		},
	},
	)
	if err != nil {
		log.Fatalf("Failed to create the Jaeger exporter: %v", err)
	}

	trace.RegisterExporter(je)
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	ctx, rootSpan := trace.StartSpan(context.Background(), "root")

	withOcsql(ctx)
	withGormOcsql(ctx)
	withLegacyGormPlugin(ctx)

	rootSpan.End()

	je.Flush()
}

type User struct {
	ID     uint64
	Name   string
	ApiKey string
}

// https://v1.gorm.io/docs/
func withLegacyGormPlugin(ctx context.Context) {
	gdb, err := legacygorm.Open("mysql", mysqlconf.FormatDSN())
	if err != nil {
		log.Println("failed to open gorm")
		return
	}
	defer gdb.Close()
	ocgorm.RegisterCallbacks(gdb)

	ctx, mainSpan := trace.StartSpan(ctx, "with_legacy_gorm_plugin", trace.WithSpanKind(trace.SpanKindServer))
	defer mainSpan.End()

	for i := 0; i < 10; i++ {
		var user User
		if err := ocgorm.WithContext(ctx, gdb).First(&user, "id = ?", i).Error; err != nil {
			if !errors.Is(err, legacygorm.ErrRecordNotFound) {
				log.Println(err)
				return
			}
		}
	}
}

func withGormOcsql(ctx context.Context) {
	driverName, err := ocsql.Register("mysql", ocsql.WithAllTraceOptions(), ocsql.WithDisableErrSkip(true))
	if err != nil {
		log.Printf("failed to register ocsql: %v", err)
		return
	}
	db, err := sql.Open(driverName, mysqlconf.FormatDSN())
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	gdb, err := gorm.Open(gmysql.New(gmysql.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		log.Println("failed to open gorm")
		return
	}

	ctx, mainSpan := trace.StartSpan(ctx, "with_gorm_ocsql", trace.WithSpanKind(trace.SpanKindServer))
	defer mainSpan.End()

	var user User
	if err := gdb.WithContext(ctx).First(&user, "id = ?", 1).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Println(err)
			return
		}
	}
}

// https://github.com/opencensus-integrations/ocsql
func withOcsql(ctx context.Context) {
	driverName, err := ocsql.Register("mysql", ocsql.WithAllTraceOptions())
	if err != nil {
		log.Printf("failed to register ocsql: %v", err)
		return
	}

	db, err := sql.Open(driverName, mysqlconf.FormatDSN())
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	ctx, mainSpan := trace.StartSpan(ctx, "with_ocsql", trace.WithSpanKind(trace.SpanKindServer))
	for i := 0; i < 10; i++ {
		var name string
		if err := db.QueryRowContext(ctx, "SELECT name FROM users").Scan(&name); err != nil {
			log.Fatalf("failed to query: %v", err)
		}
	}
	mainSpan.End()
}
