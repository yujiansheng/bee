// Copyright 2013 bee authors
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package main

import (
	"database/sql"
	"os"
	"os/exec"
	"path"
	"strings"
)

var cmdMigrate = &Command{
	UsageLine: "migrate [Command]",
	Short:     "run database migrations",
	Long: `
bee migrate
    run all outstanding migrations

bee migrate rollback
    rollback the last migration operation

bee migrate reset
    rollback all migrations

bee migrate refresh
    rollback all migrations and run them all again
`,
}

const (
	TMP_DIR = "temp"
)

func init() {
	cmdMigrate.Run = runMigration
}

func runMigration(cmd *Command, args []string) {
	//curpath, _ := os.Getwd()

	gopath := os.Getenv("GOPATH")
	Debugf("gopath:%s", gopath)
	if gopath == "" {
		ColorLog("[ERRO] $GOPATH not found\n")
		ColorLog("[HINT] Set $GOPATH in your environment vairables\n")
		os.Exit(2)
	}

	if len(args) == 0 {
		// run all outstanding migrations
		ColorLog("[INFO] Running all outstanding migrations\n")
		migrateUpdate()
	} else {
		mcmd := args[0]
		switch mcmd {
		case "rollback":
			ColorLog("[INFO] Rolling back the last migration operation\n")
			migrateRollback()
		case "reset":
			ColorLog("[INFO] Reseting all migrations\n")
			migrateReset()
		case "refresh":
			ColorLog("[INFO] Refreshing all migrations\n")
			migrateReset()
		default:
			ColorLog("[ERRO] Command is missing\n")
			os.Exit(2)
		}
		ColorLog("[SUCC] Migration successful!\n")
	}
}

func checkForSchemaUpdateTable(driver string, connStr string) {
	db, err := sql.Open(driver, connStr)
	if err != nil {
		ColorLog("[ERRO] Could not connect to %s: %s\n", driver, connStr)
		os.Exit(2)
	}
	defer db.Close()
	if rows, err := db.Query("SHOW TABLES LIKE 'migrations'"); err != nil {
		ColorLog("[ERRO] Could not show migrations table: %s\n", err)
		os.Exit(2)
	} else if !rows.Next() {
		// no migrations table, create anew
		ColorLog("[INFO] Creating 'migrations' table...\n")
		if _, err := db.Query(MYSQL_MIGRATION_DDL); err != nil {
			ColorLog("[ERRO] Could not create migrations table: %s\n", err)
			os.Exit(2)
		}
	}
	// checking that migrations table schema are expected
	if rows, err := db.Query("DESC migrations"); err != nil {
		ColorLog("[ERRO] Could not show columns of migrations table: %s\n", err)
		os.Exit(2)
	} else {
		for rows.Next() {
			var fieldBytes, typeBytes, nullBytes, keyBytes, defaultBytes, extraBytes []byte
			if err := rows.Scan(&fieldBytes, &typeBytes, &nullBytes, &keyBytes, &defaultBytes, &extraBytes); err != nil {
				ColorLog("[ERRO] Could not read column information: %s\n", err)
				os.Exit(2)
			}
			fieldStr, typeStr, nullStr, keyStr, defaultStr, extraStr :=
				string(fieldBytes), string(typeBytes), string(nullBytes), string(keyBytes), string(defaultBytes), string(extraBytes)
			if fieldStr == "id_migration" {
				if keyStr != "PRI" || extraStr != "auto_increment" {
					ColorLog("[ERRO] Column migration.id_migration type mismatch: KEY: %s, EXTRA: %s\n", keyStr, extraStr)
					ColorLog("[HINT] Expecting KEY: PRI, EXTRA: auto_increment\n")
					os.Exit(2)
				}
			} else if fieldStr == "file" {
				if !strings.HasPrefix(typeStr, "varchar") || nullStr != "YES" {
					ColorLog("[ERRO] Column migration.file type mismatch: TYPE: %s, NULL: %s\n", typeStr, nullStr)
					ColorLog("[HINT] Expecting TYPE: varchar, NULL: YES\n")
					os.Exit(2)
				}

			} else if fieldStr == "created_at" {
				if typeStr != "timestamp" || defaultStr != "CURRENT_TIMESTAMP" {
					ColorLog("[ERRO] Column migration.file type mismatch: TYPE: %s, DEFAULT: %s\n", typeStr, defaultStr)
					ColorLog("[HINT] Expecting TYPE: timestamp, DEFAULT: CURRENT_TIMESTAMP\n")
					os.Exit(2)
				}
			}
		}
	}
}

func createTempMigrationDir(path string) {
	if err := os.MkdirAll(path, 0777); err != nil {
		ColorLog("[ERRO] Could not create path: %s\n", err)
		os.Exit(2)
	}
}

func writeMigrationSourceFile(filename string, driver string, connStr string) {
	if f, err := os.OpenFile(filename+".go", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0666); err != nil {
		ColorLog("[ERRO] Could not create file: %s\n", err)
		os.Exit(2)
	} else {
		content := strings.Replace(MIGRATION_MAIN_TPL, "{{DBDriver}}", driver, -1)
		content = strings.Replace(content, "{{ConnStr}}", connStr, -1)
		content = strings.Replace(content, "{{CurrTime}}", "123", -1)
		if _, err := f.WriteString(content); err != nil {
			ColorLog("[ERRO] Could not write to file: %s\n", err)
			os.Exit(2)
		}
		f.Close()
	}
}

func buildMigrationBinary(filename string) {
	cmd := exec.Command("go", "build", "-o", filename, filename+".go")
	if err := cmd.Run(); err != nil {
		ColorLog("[ERRO] Could not build migration binary: %s\n", err)
		os.Exit(2)
	}
}

func runMigrationBinary(filename string) {
	cmd := exec.Command("./" + filename)
	if out, err := cmd.CombinedOutput(); err != nil {
		ColorLog("[ERRO] Could not run migration binary\n")
		os.Exit(2)
	} else {
		ColorLog("[INFO] %s", string(out))
	}
}

func cleanUpMigrationFiles(tmpPath string) {
	if err := os.RemoveAll(tmpPath); err != nil {
		ColorLog("[ERRO] Could not remove temporary migration directory: %s\n", err)
		os.Exit(2)
	}
}

func migrateUpdate() {
	connStr := "root:@tcp(127.0.0.1:3306)/sgfas?charset=utf8"
	checkForSchemaUpdateTable("mysql", connStr)
	filename := path.Join(TMP_DIR, "super")
	createTempMigrationDir(TMP_DIR)
	writeMigrationSourceFile(filename, "mysql", connStr)
	buildMigrationBinary(filename)
	runMigrationBinary(filename)
	cleanUpMigrationFiles(TMP_DIR)
}

func migrateRollback() {
}

func migrateReset() {
}

func migrateRefresh() {
	migrateReset()
	migrateUpdate()
}

const (
	MIGRATION_MAIN_TPL = `package main

import(
	"github.com/astaxie/beego/orm"
	"github.com/astaxie/beego/migration"
)

func init(){
	orm.RegisterDb("default", "{{DBDriver}}","{{ConnStr}}")
}

func main(){
	migration.Upgrade({{CurrTime}})
	//migration.Rollback()
	//migration.Reset()
	//migration.Refresh()
}

`
	MYSQL_MIGRATION_DDL = `
CREATE TABLE migrations (
	id_migration int(10) unsigned NOT NULL AUTO_INCREMENT,
	file varchar(255) DEFAULT NULL,
	created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
	statements text,
	PRIMARY KEY (id_migration)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 
`
)
