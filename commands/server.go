/* Vuls - Vulnerability Scanner
Copyright (C) 2016  Future Architect, Inc. Japan.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package commands

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	// "github.com/future-architect/vuls/Server"

	c "github.com/future-architect/vuls/config"
	"github.com/future-architect/vuls/oval"
	"github.com/future-architect/vuls/report"
	"github.com/future-architect/vuls/server"
	"github.com/future-architect/vuls/util"
	"github.com/google/subcommands"
	cvelog "github.com/kotakanbe/go-cve-dictionary/log"
)

// ServerCmd is subcommand for server
type ServerCmd struct {
	configPath string
	listen     string
	cvelDict   c.GoCveDictConf
	ovalDict   c.GovalDictConf
	gostConf   c.GostConf
}

// Name return subcommand name
func (*ServerCmd) Name() string { return "server" }

// Synopsis return synopsis
func (*ServerCmd) Synopsis() string { return "Server" }

// Usage return usage
func (*ServerCmd) Usage() string {
	return `Server:
	Server
		[-lang=en|ja]
		[-config=/path/to/config.toml]
		[-log-dir=/path/to/log]
		[-cvss-over=7]
		[-diff]
		[-ignore-unscored-cves]
		[-ignore-unfixed]
		[-to-email]
		[-to-slack]
		[-to-stride]
		[-to-hipchat]
		[-to-chatwork]
		[-to-localfile]
		[-to-s3]
		[-to-azure-blob]
		[-format-json]
		[-format-xml]
		[-format-one-email]
		[-format-one-line-text]
		[-format-list]
		[-format-full-text]
		[-http-proxy=http://192.168.0.1:8080]
		[-debug]
		[-debug-sql]
		[-listen=localhost:5515]
		[-cvedb-type=sqlite3|mysql|postgres|redis]
		[-cvedb-path=/path/to/cve.sqlite3]
		[-cvedb-url=http://127.0.0.1:1323 or DB connection string]
		[-ovaldb-type=sqlite3|mysql|redis]
		[-ovaldb-path=/path/to/oval.sqlite3]
		[-ovaldb-url=http://127.0.0.1:1324 or DB connection string]
		[-gostdb-type=sqlite3|mysql|redis]
		[-gostdb-path=/path/to/gost.sqlite3]
		[-gostdb-url=http://127.0.0.1:1325 or DB connection string]

		[RFC3339 datetime format under results dir]
`
}

// SetFlags set flag
func (p *ServerCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.Conf.Lang, "lang", "en", "[en|ja]")
	f.BoolVar(&c.Conf.Debug, "debug", false, "debug mode")
	f.BoolVar(&c.Conf.DebugSQL, "debug-sql", false, "SQL debug mode")

	wd, _ := os.Getwd()
	defaultConfPath := filepath.Join(wd, "config.toml")
	f.StringVar(&p.configPath, "config", defaultConfPath, "/path/to/toml")

	defaultResultsDir := filepath.Join(wd, "results")
	f.StringVar(&c.Conf.ResultsDir, "results-dir", defaultResultsDir, "/path/to/results")

	defaultLogDir := util.GetDefaultLogDir()
	f.StringVar(&c.Conf.LogDir, "log-dir", defaultLogDir, "/path/to/log")

	f.Float64Var(&c.Conf.CvssScoreOver, "cvss-over", 0,
		"-cvss-over=6.5 means Servering CVSS Score 6.5 and over (default: 0 (means Server all))")

	f.BoolVar(&c.Conf.IgnoreUnscoredCves, "ignore-unscored-cves", false,
		"Don't Server the unscored CVEs")

	f.BoolVar(&c.Conf.IgnoreUnfixed, "ignore-unfixed", false,
		"Don't Server the unfixed CVEs")

	f.StringVar(&c.Conf.HTTPProxy, "http-proxy", "",
		"http://proxy-url:port (default: empty)")

	f.BoolVar(&c.Conf.FormatJSON, "format-json", false, "JSON format")

	f.BoolVar(&c.Conf.ToLocalFile, "to-localfile", false, "Write report to localfile")
	f.StringVar(&p.listen, "listen", "localhost:5515",
		"host:port (default: localhost:5515)")

	f.StringVar(&p.cvelDict.Type, "cvedb-type", "sqlite3",
		"DB type of go-cve-dictionary (sqlite3, mysql, postgres or redis)")
	f.StringVar(&p.cvelDict.SQLite3Path, "cvedb-path", "", "/path/to/sqlite3")
	f.StringVar(&p.cvelDict.URL, "cvedb-url", "",
		"http://go-cve-dictionary.com:1323 or DB connection string")

	f.StringVar(&p.ovalDict.Type, "ovaldb-type", "",
		"DB type of goval-dictionary (sqlite3, mysql, postgres or redis)")
	f.StringVar(&p.ovalDict.SQLite3Path, "ovaldb-path", "", "/path/to/sqlite3")
	f.StringVar(&p.ovalDict.URL, "ovaldb-url", "",
		"http://goval-dictionary.com:1324 or DB connection string")

	f.StringVar(&p.gostConf.Type, "gostdb-type", "",
		"DB type of gost (sqlite3, mysql, postgres or redis)")
	f.StringVar(&p.gostConf.SQLite3Path, "gostdb-path", "", "/path/to/sqlite3")
	f.StringVar(&p.gostConf.URL, "gostdb-url", "",
		"http://gost.com:1325 or DB connection string")
}

// Execute execute
func (p *ServerCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	util.Log = util.NewCustomLogger(c.ServerInfo{})
	cvelog.SetLogger(c.Conf.LogDir, false, c.Conf.Debug, false)

	c.Conf.CveDict.Overwrite(p.cvelDict)
	c.Conf.OvalDict.Overwrite(p.ovalDict)
	c.Conf.Gost.Overwrite(p.gostConf)

	util.Log.Info("Validating config...")
	if !c.Conf.ValidateOnReport() {
		return subcommands.ExitUsageError
	}

	if err := report.CveClient.CheckHealth(); err != nil {
		util.Log.Errorf("CVE HTTP server is not running. err: %s", err)
		util.Log.Errorf("Run go-cve-dictionary as server mode before Servering or run with -cvedb-path option")
		return subcommands.ExitFailure
	}
	if c.Conf.CveDict.URL != "" {
		util.Log.Infof("cve-dictionary: %s", c.Conf.CveDict.URL)
	} else {
		if c.Conf.CveDict.Type == "sqlite3" {
			util.Log.Infof("cve-dictionary: %s", c.Conf.CveDict.SQLite3Path)
		}
	}

	if c.Conf.OvalDict.URL != "" {
		util.Log.Infof("oval-dictionary: %s", c.Conf.OvalDict.URL)
		err := oval.Base{}.CheckHTTPHealth()
		if err != nil {
			util.Log.Errorf("OVAL HTTP server is not running. err: %s", err)
			util.Log.Errorf("Run goval-dictionary as server mode before Servering or run with -ovaldb-path option")
			return subcommands.ExitFailure
		}
	} else {
		if c.Conf.OvalDict.Type == "sqlite3" {
			util.Log.Infof("oval-dictionary: %s", c.Conf.OvalDict.SQLite3Path)
		}
	}

	dbclient, locked, err := report.NewDBClient(report.DBClientConf{
		CveDictCnf:  c.Conf.CveDict,
		OvalDictCnf: c.Conf.OvalDict,
		GostCnf:     c.Conf.Gost,
		DebugSQL:    c.Conf.DebugSQL,
	})
	if locked {
		util.Log.Errorf("SQLite3 is locked. Close other DB connections and try again: %s", err)
		return subcommands.ExitFailure
	}

	if err != nil {
		util.Log.Errorf("Failed to init DB Clients: %s", err)
		return subcommands.ExitFailure
	}

	defer dbclient.CloseDB()

	http.Handle("/vuls", server.VulsHandler{DBclient: *dbclient})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	})
	util.Log.Infof("Listening on %s", p.listen)
	if err := http.ListenAndServe(p.listen, nil); err != nil {
		util.Log.Errorf("Failed to start server: %s", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}
