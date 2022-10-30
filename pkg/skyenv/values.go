// Package skyenv defines variables and constants for different operating systems
package skyenv

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
	"runtime"

	"github.com/bitfield/script"
	"github.com/google/uuid"
	"github.com/jaypipes/ghw"
	"github.com/skycoin/dmsg/pkg/dmsg"

	"github.com/skycoin/skywire-utilities/pkg/buildinfo"
	"github.com/skycoin/skywire-utilities/pkg/cipher"
)

const (
	// ConfigName is the default config name. Updated by setting config file path.
	ConfigName = "skywire-config.json"

	// DMSGHTTPName is the default dmsghttp config name
	DMSGHTTPName = "dmsghttp-config.json"
)

// Constants for skywire root directories.
// Dmsg port constants.
// TODO(evanlinjin): Define these properly. These are currently random.
const (
	DmsgCtrlPort           uint16 = 7                        // Listening port for dmsgctrl protocol (similar to TCP Echo Protocol).
	DmsgSetupPort          uint16 = 36                       // Listening port of a setup node.
	DmsgHypervisorPort     uint16 = 46                       // Listening port of a hypervisor for incoming RPC visor connections over dmsg.
	DmsgTransportSetupPort uint16 = 47                       // Listening port for transport setup RPC over dmsg.
	DmsgHTTPPort           uint16 = dmsg.DefaultDmsgHTTPPort // Listening port for dmsghttp logserver.
	DmsgAwaitSetupPort     uint16 = 136                      // Listening port of a visor for setup operations.
)

// Transport port constants.
const (
	TransportPort     uint16 = 45 // Listening port of a visor for incoming transports.
	PublicAutoconnect        = true
)

// Dmsgpty constants.
const (
	DmsgPtyPort   uint16 = 22
	DmsgPtyCLINet        = "unix"
)

// Skywire-TCP constants.
const (
	STCPAddr = ":7777"
)

// Default skywire app constants.
const (
	SkychatName        = "skychat"
	SkychatPort uint16 = 1
	SkychatAddr        = ":8001"

	SkysocksName        = "skysocks"
	SkysocksPort uint16 = 3

	SkysocksClientName        = "skysocks-client"
	SkysocksClientPort uint16 = 13
	SkysocksClientAddr        = ":1080"

	VPNServerName        = "vpn-server"
	VPNServerPort uint16 = 44

	VPNClientName = "vpn-client"
	// TODO(darkrengarius): this one's not needed for the app to run but lack of it causes errors
	VPNClientPort uint16 = 43
)

// RPC constants.
const (
	RPCAddr             = "localhost:3435"
	RPCTimeout          = 20 * time.Second
	TransportRPCTimeout = 1 * time.Minute
	UpdateRPCTimeout    = 6 * time.Hour // update requires huge timeout
)

// Default skywire app server and discovery constants
const (
	AppSrvAddr                = "localhost:5505"
	ServiceDiscUpdateInterval = time.Minute
	AppBinPath                = "./apps"
	LogLevel                  = "info"
)

// Routing constants
const (
	TpLogStore = "transport_logs"
	Custom     = "custom"
)

// Local constants
const (
	LocalPath = "./local"
)

// Default hypervisor constants
const (
	HypervisorDB      = ".skycoin/hypervisor/users.db"
	EnableAuth        = false
	PackageEnableAuth = true
	EnableTLS         = false
	TLSKey            = "./ssl/key.pem"
	TLSCert           = "./ssl/cert.pem"
)

const (
	// IPCShutdownMessageType sends IPC shutdown message type
	IPCShutdownMessageType = 68
)

const (
	//IsPublic advertises the visor in the service discovery
	IsPublic = false
)

// PkgConfig struct contains paths specific to the linux packages
type PkgConfig struct {
	Launcher   `json:"launcher"`
	LocalPath  string `json:"local_path"`
	Hypervisor `json:"hypervisor"`
	//		TLSCertFile string `json:"tls_cert_file"`
	//		TLSKeyFile  string `json:"tls_key_file"`
}

// Launcher struct contains the BinPath specific to the linux packages
type Launcher struct {
	BinPath string `json:"bin_path"`
}

// Hypervisor struct contains Hypervisor paths specific to the linux packages
type Hypervisor struct {
	DbPath     string `json:"db_path"`
	EnableAuth bool   `json:"enable_auth"`
}

// DmsgPtyWhiteList gets dmsgpty whitelist path for installed Skywire.
func DmsgPtyWhiteList() string {
	return filepath.Join(SkywirePath, "dmsgpty", "whitelist.json")
}

// MustPK unmarshals string PK to cipher.PubKey. It panics if unmarshaling fails.
func MustPK(pk string) cipher.PubKey {
	var sPK cipher.PubKey
	if err := sPK.UnmarshalText([]byte(pk)); err != nil {
		panic(err)
	}

	return sPK
}

// Version gets the version of the installation for the config
func Version() string {
	u := buildinfo.Version()
	v := u
	if u == "unknown" {
		//check for .git folder for versioning
		if _, err := os.Stat(".git"); err == nil {
			//attempt to version from git sources
			if _, err = exec.LookPath("git"); err == nil {
				if v, err = script.Exec(`git describe`).String(); err == nil {
					v = strings.ReplaceAll(v, "\n", "")
					v = strings.Split(v, "-")[0]
				}
			}
		}
	}
	return v
}

// HomePath gets the current user's home folder
func HomePath() string {
	dir, _ := os.UserHomeDir() //nolint
	return dir
}

// Config returns either UserConfig or PackageConfig based on permissions
func Config() PkgConfig {
	if IsRoot() {
		return PackageConfig()
	}
	return UserConfig()
}

// IsRoot checks for root permissions
func IsRoot() bool {
	userLvl, _ := user.Current() //nolint
	return userLvl.Username == "root"
}

// Survey system hardware survey struct
type Survey struct {
	UUID         uuid.UUID        `json:"uuid,omitempty"`
	PubKey       cipher.PubKey    `json:"public_key,omitempty"`
	OS           string           `json:"os,omitempty"`
	Architecture string           `json:"arch,omitempty"`
	IP           cipher.PubKey    `json:"ip_address,omitempty"`
	Disks        *ghw.BlockInfo   `json:"disks,omitempty"`
	Product      *ghw.ProductInfo `json:"product_info,omitempty"`
	Memory       *ghw.MemoryInfo  `json:"memory_info,omitempty"`
}

// SurveyFile is the name of the survey file
const SurveyFile string = "system.json"

// RewardFile is the name of the file containing skycoin rewards address and privacy setting
const RewardFile string = "reward.txt"

// SystemSurvey returns system survey
func SystemSurvey() (Survey, error) {
	operatingSystem :=  runtime.GOOS
	systemArchitecture :=  runtime.GOOS
	//	ip, err := externalip.DefaultConsensus(nil, nil).ExternalIP()
	//	if err == nil {
	//		fmt.Println(ip.String()) // print IPv4/IPv6 in string format
	//	}
	disks, err := ghw.Block()
	if err != nil {
		return Survey{}, err
	}
	product, err := ghw.Product()
	if err != nil {
		return Survey{}, err
	}
	memory, err := ghw.Memory()
	if err != nil {
		return Survey{}, err
	}
	s := Survey{
		OS:	operatingSystem,
		Architecture: systemArchitecture,
		UUID:    uuid.New(),
		Disks:   disks,
		Product: product,
		Memory:  memory,
	}
	return s, nil
}
