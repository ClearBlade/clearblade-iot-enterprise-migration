package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	cb "github.com/clearblade/Go-SDK"
	cbiotcore "github.com/clearblade/go-iot"
)

const TotalWorkers = 10

var (
	Args           DeviceMigratorArgs
	cbCtx          context.Context
	iotCoreService *cbiotcore.Service
	cbDevClient    *cb.DevClient
)

var (
	colorCyan   = "\033[36m"
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
)

type DeviceMigratorArgs struct {
	// ClearBlade IoT Core specific flags
	cbServiceAccount string
	cbRegistryName   string
	cbRegistryRegion string

	//CB Enterprise Flags
	cbEnterpriseUrl    string
	cbEnterpriseMsgUrl string
	cbSystemKey        string
	cbSystemSecret     string
	cbDevEmail         string
	cbDevPwd           string

	// Optional flags
	devicesCsvFile   string
	columnsCsvFile   string
	deviceType       string
	pageSize         int
	updatePublicKeys bool
	silentMode       bool
	createDeviceRole bool
}

func initMigrationFlags() {
	//CB IoT Core Flags
	flag.StringVar(&Args.cbServiceAccount, "cbServiceAccount", "", "Path to a ClearBlade service account file. See https://clearblade.atlassian.net/wiki/spaces/IC/pages/2240675843/Add+service+accounts+to+a+project (Required)")
	flag.StringVar(&Args.cbRegistryName, "cbRegistryName", "", "ClearBlade Registry Name (Required)")
	flag.StringVar(&Args.cbRegistryRegion, "cbRegistryRegion", "", "ClearBlade Registry Region (Required)")

	//CB Enterprise Flags
	flag.StringVar(&Args.cbEnterpriseUrl, "cbEnterpriseUrl", "", "ClearBlade IoT Enterprise Url (Required)")
	flag.StringVar(&Args.cbEnterpriseMsgUrl, "cbEnterpriseMsgUrl", "", "ClearBlade IoT Enterprise Messaging Url (Required)")
	flag.StringVar(&Args.cbSystemKey, "cbSystemKey", "", "ClearBlade IoT Enterprise System Key (Required)")
	flag.StringVar(&Args.cbSystemSecret, "cbSystemSecret", "", "ClearBlade IoT Enterprise System Secret (Required)")
	flag.StringVar(&Args.cbDevEmail, "cbDevEmail", "", "ClearBlade IoT Enterprise developer e-mail (Required)")
	flag.StringVar(&Args.cbDevPwd, "cbDevPwd", "", "ClearBlade IoT Enterprise developer password (Required)")

	// Optional
	flag.StringVar(&Args.devicesCsvFile, "devicesCsv", "", "Devices CSV file path")
	flag.StringVar(&Args.columnsCsvFile, "columnMapCsv", "", "Column Map CSV file path")
	flag.StringVar(&Args.deviceType, "deviceType", "", "Device type")
	flag.IntVar(&Args.pageSize, "pageSize", 100, "Page Size")
	flag.BoolVar(&Args.updatePublicKeys, "updatePublicKeys", true, "Replace existing keys of migrated devices. Default is true")
	flag.BoolVar(&Args.silentMode, "silentMode", false, "Run this tool in silent (non-interactive) mode. Default is false")
	flag.BoolVar(&Args.createDeviceRole, "createDeviceRole", false, "Should the device roles and permissions be created")
}

func main() {
	// Init & Parse migration Flags
	initMigrationFlags()
	flag.Parse()

	if len(os.Args) == 1 {
		log.Fatalln("No flags supplied. Use clearblade-iot-enterprise-migration --help to view details.")
	}

	if os.Args[1] == "version" {
		fmt.Printf("%s\n", cbIotEnterpriseMigrationVersion)
		os.Exit(0)
	}

	if runtime.GOOS == "windows" {
		colorCyan = ""
		colorReset = ""
		colorGreen = ""
		colorYellow = ""
		colorRed = ""
	}

	// Validate if all required CB flags are provided
	validateCBFlags()
	validateEnterpriseFlags()

	fmt.Println(string(colorGreen), "\n\u2713 All Flags validated!", string(colorReset))

	//Create the ClearBlade IoT Core services
	var err error

	cbCtx = context.Background()
	iotCoreService, err = cbiotcore.NewService(cbCtx)
	if err != nil {
		fmt.Println(string(colorRed), "\n\u2715 Error creating IoT core service interface: %s", err.Error(), string(colorReset))
		os.Exit(0)
	}

	// GetRegistryCredentials
	regDetails, err := cbiotcore.GetRegistryCredentials(Args.cbRegistryName, Args.cbRegistryRegion, iotCoreService)
	if err != nil {
		fmt.Println(string(colorRed), "\n\u2715 Error retrieving registry credentials: %s! Please check if -cbRegistryName and/or -cbRegistryRegion flags are set correctly.", err.Error(), string(colorReset))
		os.Exit(0)
	}

	if regDetails.SystemKey == "" {
		fmt.Println(string(colorRed), "\n\u2715 Unable to fetch ClearBlade registry Details! Please check if -cbRegistryName and/or -cbRegistryRegion flags are set correctly.", string(colorReset))
		os.Exit(0)
	}

	// Authenticate Clearblade User account
	cbDevClient, err = authenticateCbEnterprise(&Args)
	if err != nil {
		fmt.Println(string(colorRed), "\n\u2715 Error authenticating with ClearBlade IoT Enterprise: %s", err.Error(), string(colorReset))
		os.Exit(0)
	}

	//GetDeviceCount
	deviceCount, err := getDeviceCount(regDetails, Args.cbRegistryName, Args.cbRegistryRegion, iotCoreService)
	if err != nil {
		fmt.Println(string(colorRed), "\n\u2715 Error retrieving registry device count: %s!", err.Error(), string(colorReset))
		os.Exit(0)
	}

	if deviceCount > 0 {
		migrateDevices(deviceCount)
	} else {
		fmt.Println(string(colorRed), "\n\n\u2715 No devices in registry. Skipping migration.", string(colorReset))
	}
}

func validateCBFlags() {

	if Args.cbServiceAccount == "" {
		if Args.silentMode {
			log.Fatalln("-cbServiceAccount is a required paramter")
		}

		value, err := readInput("Enter path to ClearBlade service account file. See https://clearblade.atlassian.net/wiki/spaces/IC/pages/2240675843/Add+service+accounts+to+a+project for more info: ")
		if err != nil {
			log.Fatalln("Error reading service account: ", err)
		}
		Args.cbServiceAccount = value
	}

	// validate that path to service account file exists
	if _, err := os.Stat(Args.cbServiceAccount); errors.Is(err, os.ErrNotExist) {
		log.Fatalf("Could not locate service account file %s. Please make sure the path is correct\n", Args.cbServiceAccount)
	}

	err := os.Setenv("CLEARBLADE_CONFIGURATION", Args.cbServiceAccount)
	if err != nil {
		log.Fatalln("Failed to set CLEARBLADE_CONFIGURATION env variable", err.Error())
	}

	if Args.cbRegistryName == "" {
		if Args.silentMode {
			log.Fatalln("-cbRegistryName is required parameter")
		}
		value, err := readInput("Enter ClearBlade Registry Name: ")
		if err != nil {
			log.Fatalln("Error reading registry name: ", err)
		}
		Args.cbRegistryName = value
	}

	if Args.cbRegistryRegion == "" {
		if Args.silentMode {
			log.Fatalln("-cbRegistryRegion is required parameter")
		}
		value, err := readInput("Enter ClearBlade Registry Region: ")
		if err != nil {
			log.Fatalln("Error reading ClearBlade registry region: ", err)
		}

		Args.cbRegistryRegion = value
	}

	if Args.devicesCsvFile == "" {
		if Args.silentMode {
			return
		}
		value, err := readInput("Enter Devices CSV file path (By default all devices from the registry will be migrated. Press enter to skip!): ")
		if err != nil {
			log.Fatalln("Error reading service account file path: ", err)
		}
		Args.devicesCsvFile = value
	}
}

func validateEnterpriseFlags() {
	if Args.cbEnterpriseUrl == "" {
		if Args.silentMode {
			log.Fatalln("-cbEnterpriseUrl is a required paramter")
		}

		value, err := readInput("Enter the URL of the IoT Enterprise instance the devices will be migrated to: ")
		if err != nil {
			log.Fatalln("Error reading IoT Enterprise URL: ", err)
		}
		Args.cbEnterpriseUrl = value
	}

	if Args.cbEnterpriseMsgUrl == "" {
		if Args.silentMode {
			log.Fatalln("-cbEnterpriseMsgUrl is a required paramter")
		}

		value, err := readInput("Enter the messaging URL of the IoT Enterprise instance the devices will be migrated to: ")
		if err != nil {
			log.Fatalln("Error reading IoT Enterprise messaging url: ", err)
		}
		Args.cbEnterpriseMsgUrl = value
	}

	if Args.cbSystemKey == "" {
		if Args.silentMode {
			log.Fatalln("-cbSystemKey is a required paramter")
		}

		value, err := readInput("Enter the system key of the IoT Enterprise system the devices will be migrated to: ")
		if err != nil {
			log.Fatalln("Error reading system key: ", err)
		}
		Args.cbSystemKey = value
	}

	if Args.cbSystemSecret == "" {
		if Args.silentMode {
			log.Fatalln("-cbSystemSecret is a required paramter")
		}

		value, err := readInput("Enter the system secret of the IoT Enterprise system the devices will be migrated to: ")
		if err != nil {
			log.Fatalln("Error reading system secret: ", err)
		}
		Args.cbSystemSecret = value
	}

	if Args.cbDevEmail == "" {
		if Args.silentMode {
			log.Fatalln("-cbDevEmail is a required paramter")
		}

		value, err := readInput("Enter the developer email address that will be used to authenticate with the target IoT Enterprise system: ")
		if err != nil {
			log.Fatalln("Error developer email address: ", err)
		}
		Args.cbDevEmail = value
	}

	if Args.cbDevPwd == "" {
		if Args.silentMode {
			log.Fatalln("-cbDevPwd is a required paramter")
		}

		value, err := readPassword("Enter the developer password that will be used to authenticate with the target IoT Enterprise system: ")
		if err != nil {
			log.Fatalln("Error developer password: ", err)
		}
		Args.cbDevPwd = value
	}

	if Args.columnsCsvFile == "" {
		if Args.silentMode {
			return
		}
		value, err := readInput("Enter the path to a CSV file containing column mappings (Press enter to skip!): ")
		if err != nil {
			log.Fatalln("Error reading column map CSV file path: ", err)
		}
		Args.columnsCsvFile = value
	}

	if Args.deviceType == "" {
		if Args.silentMode {
			return
		}
		value, err := readInput("Enter the device type to assign to each migrated device (Press enter to skip!): ")
		if err != nil {
			log.Fatalln("Error reading device type: ", err)
		}
		Args.deviceType = value
	}
}

func migrateDevices(deviceCount int) {
	fmt.Println(string(colorCyan), "\n\n================= Starting Device Migration =================\n\nRunning Version: ", cbIotEnterpriseMigrationVersion, "\n\n", string(colorReset))
	fmt.Println(string(colorCyan), "\nPreparing Device Migration\n", string(colorReset))

	// Fetch devices from the given registry
	errorLogs := migrateDevicesFromCbIotCore(deviceCount)
	if len(errorLogs) > 0 {
		fmt.Println("Invoking generateFailedDevicesCSV")
		if err := generateFailedDevicesCSV(errorLogs); err != nil {
			log.Fatalln(err)
		}
	}

	fmt.Println(string(colorGreen), "\n\n\u2713 Done!", string(colorReset))
}
