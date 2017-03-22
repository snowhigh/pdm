package cmd

import (
	"fmt"
        "log"
	"os"
        "time"
	"database/sql"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/oleksandr/bonjour"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(scanClusterCommand)
}

var scanClusterCommand = &cobra.Command{
        Use:   "scan-cluster",
        Short: "scan for k8sup clusters",
        RunE: func(cmd *cobra.Command, args []string) error {
		// Open db for read/write scan result
		db, err := sql.Open("sqlite3", "/tmp/cdxctl.db")
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()
		sqlStmt := `create table if not exists cluster(
			vlan_id integer default 0,
			ipv4 varchar(50),
			hostname varchar(100),
			cluster varchar(100),
			last_update TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			unique(vlan_id, ipv4) on conflict replace
		);`
		_, err = db.Exec(sqlStmt)
		if err != nil {
			log.Printf("%q: %s\n", err, sqlStmt)
			return err
		}

		scanCluster(db)
		log.Printf("Done")
		return nil
        },
}

func scanCluster(db *sql.DB) {
	resolver, err := bonjour.NewResolver(nil)
	if err != nil {
		log.Println("Failed to initialize resolver:", err.Error())
		os.Exit(1)
	}

	results := make(chan *bonjour.ServiceEntry)

	// Send the "stop browsing" signal after the desired timeout
	timeout := time.Duration(5 * time.Second)
	exitCh := make(chan bool)
	go func() {
		time.Sleep(timeout)
		go func() { resolver.Exit <- true }()
		go func() { exitCh <- true }()
	}()

	err = resolver.Browse("_etcd._tcp", "local.", results)
	if err != nil {
		log.Println("Failed to browse:", err.Error())
	}

	for {
		select {
		case e := <-results:
			// fmt.Printf("%s %s:%d %s %s\n", e.HostName, e.AddrIPv4, e.Port, e.Text, e.ServiceInstanceName())
			tx, err := db.Begin()
			if err != nil {
				log.Fatal(err)
			}
			stmt, err := tx.Prepare("insert or replace into cluster(vlan_id, ipv4, hostname, cluster) values(?,?,?,?)")
			if err != nil {
				log.Fatal(err)
			}
			defer stmt.Close()
			clusterID := "Unknown"
			for _, b := range e.Text {
				if strings.Contains(b, "clusterID") {
					clusterID = strings.Split(b, "=")[1]
				}
			}
			hostname := strings.Split(e.HostName, ".")[0]
			ipv4 := fmt.Sprintf("%s", e.AddrIPv4)
			_, err = stmt.Exec(0, ipv4, hostname, clusterID)
			if err != nil {
				log.Fatal(err)
			}
			tx.Commit()
		case <-exitCh:
			return
		}
	}
}

