// Package commands cmd/transport-discovery/root.go
package commands

import (
	"context"
	"fmt"
	"log"
	"log/syslog"
	"os"
	"strings"
	"time"

	cc "github.com/ivanpirog/coloredcobra"
	logrussyslog "github.com/sirupsen/logrus/hooks/syslog"
	"github.com/skycoin/dmsg/pkg/direct"
	"github.com/skycoin/dmsg/pkg/dmsg"
	"github.com/skycoin/dmsg/pkg/dmsghttp"
	"github.com/skycoin/skywire-utilities/pkg/buildinfo"
	"github.com/skycoin/skywire-utilities/pkg/cipher"
	"github.com/skycoin/skywire-utilities/pkg/cmdutil"
	"github.com/skycoin/skywire-utilities/pkg/httpauth"
	"github.com/skycoin/skywire-utilities/pkg/logging"
	"github.com/skycoin/skywire-utilities/pkg/metricsutil"
	"github.com/skycoin/skywire-utilities/pkg/skyenv"
	"github.com/skycoin/skywire-utilities/pkg/storeconfig"
	"github.com/skycoin/skywire-utilities/pkg/tcpproxy"
	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"github.com/skycoin/skywire-services/internal/pg"
	"github.com/skycoin/skywire-services/internal/tpdiscmetrics"
	"github.com/skycoin/skywire-services/pkg/transport-discovery/api"
	"github.com/skycoin/skywire-services/pkg/transport-discovery/store"
)

const (
	redisPrefix = "transport-discovery"
	redisScheme = "redis://"
)

var (
	addr            string
	metricsAddr     string
	redisURL        string
	redisPoolSize   int
	pgHost          string
	pgPort          string
	syslogAddr      string
	logLvl          string
	tag             string
	testing         bool
	dmsgDisc        string
	whitelistKeys   string
	testEnvironment bool
	sk              cipher.SecKey
	dmsgPort        uint16
)

func init() {
	RootCmd.Flags().StringVarP(&addr, "addr", "a", ":9091", "address to bind to\033[0m")
	RootCmd.Flags().StringVarP(&metricsAddr, "metrics", "m", "", "address to bind metrics API to\033[0m")
	RootCmd.Flags().StringVar(&redisURL, "redis", "redis://localhost:6379", "connections string for a redis store\033[0m")
	RootCmd.Flags().IntVar(&redisPoolSize, "redis-pool-size", 10, "redis connection pool size\033[0m")
	RootCmd.Flags().StringVar(&pgHost, "pg-host", "localhost", "host of postgres\033[0m")
	RootCmd.Flags().StringVar(&pgPort, "pg-port", "5432", "port of postgres\033[0m")
	RootCmd.Flags().StringVar(&syslogAddr, "syslog", "", "syslog server address. E.g. localhost:514\033[0m")
	RootCmd.Flags().StringVarP(&logLvl, "loglvl", "l", "info", "set log level one of: info, error, warn, debug, trace, panic")
	RootCmd.Flags().StringVar(&tag, "tag", "transport_discovery", "logging tag\033[0m")
	RootCmd.Flags().BoolVarP(&testing, "testing", "t", false, "enable testing to start without redis\033[0m")
	RootCmd.Flags().StringVar(&dmsgDisc, "dmsg-disc", "http://dmsgd.skywire.skycoin.com", "url of dmsg-discovery\033[0m")
	RootCmd.Flags().StringVar(&whitelistKeys, "whitelist-keys", "", "list of whitelisted keys of network monitor used for deregistration\033[0m")
	RootCmd.Flags().BoolVar(&testEnvironment, "test-environment", false, "distinguished between prod and test environment\033[0m")
	RootCmd.Flags().Var(&sk, "sk", "dmsg secret key\r")
	RootCmd.Flags().Uint16Var(&dmsgPort, "dmsgPort", dmsg.DefaultDmsgHTTPPort, "dmsg port value\r")
	var helpflag bool
	RootCmd.SetUsageTemplate(help)
	RootCmd.PersistentFlags().BoolVarP(&helpflag, "help", "h", false, "help for transport-discovery")
	RootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	RootCmd.PersistentFlags().MarkHidden("help") //nolint
}

// RootCmd contains the root command
var RootCmd = &cobra.Command{
	Use:   "tpd",
	Short: "Transport Discovery Server for skywire",
	Long: `
	┌┬┐┬─┐┌─┐┌┐┌┌─┐┌─┐┌─┐┬─┐┌┬┐ ┌┬┐┬┌─┐┌─┐┌─┐┬  ┬┌─┐┬─┐┬ ┬
	 │ ├┬┘├─┤│││└─┐├─┘│ │├┬┘ │───│││└─┐│  │ │└┐┌┘├┤ ├┬┘└┬┘
	 ┴ ┴└─┴ ┴┘└┘└─┘┴  └─┘┴└─ ┴  ─┴┘┴└─┘└─┘└─┘ └┘ └─┘┴└─ ┴ `,
	SilenceErrors:         true,
	SilenceUsage:          true,
	DisableSuggestions:    true,
	DisableFlagsInUseLine: true,
	Version:               buildinfo.Version(),
	Run: func(_ *cobra.Command, _ []string) {
		if _, err := buildinfo.Get().WriteTo(os.Stdout); err != nil {
			log.Printf("Failed to output build info: %v", err)
		}

		if !strings.HasPrefix(redisURL, redisScheme) {
			redisURL = redisScheme + redisURL
		}

		nonceStoreConfig := storeconfig.Config{
			Type:     storeconfig.Memory,
			URL:      redisURL,
			Password: storeconfig.RedisPassword(),
			PoolSize: redisPoolSize,
		}

		logger := logging.MustGetLogger(tag)
		lvl, err := logging.LevelFromString(logLvl)
		if err != nil {
			logger.Fatal("Invalid loglvl detected")
		}

		logging.SetLevel(lvl)

		var whitelistPKs []string
		if whitelistKeys != "" {
			whitelistPKs = strings.Split(whitelistKeys, ",")
		} else {
			if testEnvironment {
				whitelistPKs = strings.Split(skyenv.TestNetworkMonitorPKs, ",")
			} else {
				whitelistPKs = strings.Split(skyenv.NetworkMonitorPKs, ",")
			}
		}

		for _, v := range whitelistPKs {
			api.WhitelistPKs.Set(v)
		}

		if syslogAddr != "" {
			hook, err := logrussyslog.NewSyslogHook("udp", syslogAddr, syslog.LOG_INFO, tag)
			if err != nil {
				logger.Fatalf("Unable to connect to syslog daemon on %v", syslogAddr)
			}
			logging.AddHook(hook)
		}

		var gormDB *gorm.DB

		if !testing {
			pgUser, pgPassword, pgDatabase := storeconfig.PostgresCredential()
			dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
				pgHost,
				pgPort,
				pgUser,
				pgPassword,
				pgDatabase)

			gormDB, err = pg.Init(dsn)
			if err != nil {
				logger.Fatalf("Failed to connect to database %v", err)
			}
			logger.Printf("Database connected.")

			nonceStoreConfig.Type = storeconfig.Redis
		}

		s, err := store.New(logger, gormDB, testing)
		if err != nil {
			logger.Fatalf("Failed to create store instance: %v", err)
		}
		defer s.Close()

		ctx, cancel := cmdutil.SignalContext(context.Background(), logger)
		defer cancel()

		nonceStore, err := httpauth.NewNonceStore(ctx, nonceStoreConfig, redisPrefix)
		if err != nil {
			log.Fatal("Failed to initialize redis nonce store: ", err)
		}

		pk, err := sk.PubKey()
		if err != nil {
			logger.WithError(err).Warn("No SecKey found. Skipping serving on dmsghttp.")
		}

		metricsutil.ServeHTTPMetrics(logger, metricsAddr)

		var m tpdiscmetrics.Metrics
		if metricsAddr == "" {
			m = tpdiscmetrics.NewEmpty()
		} else {
			m = tpdiscmetrics.NewVictoriaMetrics()
		}

		var dmsgAddr string
		if !pk.Null() {
			dmsgAddr = fmt.Sprintf("%s:%d", pk.Hex(), dmsgPort)
		}

		enableMetrics := metricsAddr != ""
		tpdAPI := api.New(logger, s, nonceStore, enableMetrics, m, dmsgAddr)

		logger.Infof("Listening on %s", addr)

		go tpdAPI.RunBackgroundTasks(ctx, logger)

		go func() {
			if err := tcpproxy.ListenAndServe(addr, tpdAPI); err != nil {
				logger.Errorf("tcpproxy.ListenAndServe: %v", err)
				cancel()
			}
		}()

		if !pk.Null() {
			servers := dmsghttp.GetServers(ctx, dmsgDisc, logger)

			var keys cipher.PubKeys
			keys = append(keys, pk)
			dClient := direct.NewClient(direct.GetAllEntries(keys, servers), logger)
			config := &dmsg.Config{
				MinSessions:    0, // listen on all available servers
				UpdateInterval: dmsg.DefaultUpdateInterval,
			}

			dmsgDC, closeDmsgDC, err := direct.StartDmsg(ctx, logger, pk, sk, dClient, config)
			if err != nil {
				logger.WithError(err).Fatal("failed to start direct dmsg client.")
			}

			defer closeDmsgDC()

			go func() {
				for {
					tpdAPI.DmsgServers = dmsgDC.ConnectedServersPK()
					time.Sleep(time.Second)
				}
			}()

			go dmsghttp.UpdateServers(ctx, dClient, dmsgDisc, dmsgDC, logger)

			go func() {
				if err := dmsghttp.ListenAndServe(ctx, sk, tpdAPI, dClient, dmsg.DefaultDmsgHTTPPort, dmsgDC, logger); err != nil {
					logger.Errorf("dmsghttp.ListenAndServe: %v", err)
					cancel()
				}
			}()
		}

		<-ctx.Done()
	},
}

// Execute executes root CLI command.
func Execute() {
	cc.Init(&cc.Config{
		RootCmd:       RootCmd,
		Headings:      cc.HiBlue + cc.Bold, //+ cc.Underline,
		Commands:      cc.HiBlue + cc.Bold,
		CmdShortDescr: cc.HiBlue,
		Example:       cc.HiBlue + cc.Italic,
		ExecName:      cc.HiBlue + cc.Bold,
		Flags:         cc.HiBlue + cc.Bold,
		//FlagsDataType: cc.HiBlue,
		FlagsDescr:      cc.HiBlue,
		NoExtraNewlines: true,
		NoBottomNewline: true,
	})
	if err := RootCmd.Execute(); err != nil {
		log.Fatal("Failed to execute command: ", err)
	}
}

const help = "Usage:\r\n" +
	"  {{.UseLine}}{{if .HasAvailableSubCommands}}{{end}} {{if gt (len .Aliases) 0}}\r\n\r\n" +
	"{{.NameAndAliases}}{{end}}{{if .HasAvailableSubCommands}}\r\n\r\n" +
	"Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand)}}\r\n  " +
	"{{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}\r\n\r\n" +
	"Flags:\r\n" +
	"{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}\r\n\r\n" +
	"Global Flags:\r\n" +
	"{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}\r\n\r\n"
