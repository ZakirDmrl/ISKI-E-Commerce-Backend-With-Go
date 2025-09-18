package database

import (
	"database/sql"
	"log"
	"net/url"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func Connect(databaseURL string) error {
	var err error

	// Supabase/PgBouncer ortamlarında lib/pq'nun prepared statement kullanımını
	// devre dışı bırakmak için binary_parameters=yes ekleyelim.
	dsn := databaseURL
	if strings.Contains(databaseURL, "://") {
		if parsed, perr := url.Parse(databaseURL); perr == nil {
			q := parsed.Query()
			if q.Get("binary_parameters") == "" {
				q.Set("binary_parameters", "yes")
			}
			parsed.RawQuery = q.Encode()
			dsn = parsed.String()
		}
	} else {
		if !strings.Contains(databaseURL, "binary_parameters=") {
			if strings.TrimSpace(databaseURL) == "" {
				dsn = "binary_parameters=yes"
			} else {
				dsn = databaseURL + " binary_parameters=yes"
			}
		}
	}

	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		return err
	}

	// Connection pool ayarları
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(5 * time.Minute)

	if err = DB.Ping(); err != nil {
		return err
	}

	log.Println("Database connected successfully")
	return nil
}
