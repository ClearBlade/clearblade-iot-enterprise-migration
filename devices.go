package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	cb "github.com/clearblade/Go-SDK"
	cbiotcore "github.com/clearblade/go-iot"
)

func migrateDevicesFromCbIotCore(deviceCount int) []ErrorLog {
	errorLogs := make([]ErrorLog, 0)

	deviceService := cbiotcore.NewProjectsLocationsRegistriesDevicesService(iotCoreService)
	var devices []*cbiotcore.Device

	if Args.devicesCsvFile != "" {
		devices = fetchDevicesFromCSV(deviceService)
	} else {
		fmt.Println(string(colorGreen), "\u2713 Fetching all", deviceCount, "devices!", string(colorReset))
		devices = fetchAllDevices(deviceService)

		if len(devices) != deviceCount {
			fmt.Printf(string(colorYellow), "Warning: the following device IDs were not found - %s\n", string(colorReset))
		}
	}

	fmt.Println(string(colorGreen), "\u2713 Fetched", len(devices), "devices", string(colorReset))

	errorLogs = migrateDevicesToClearBlade(&Args, cbDevClient, devices, errorLogs)
	return errorLogs
}

func getDeviceCount(creds *cbiotcore.RegistryUserCredentials, registry string, region string, s *cbiotcore.Service) (int, error) {
	url := fmt.Sprintf("%s/api/v/1/code/%s/getNumDevicesGateways", creds.Url, creds.SystemKey)
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return 0, err
	}
	req.Close = true
	req.Header.Add("ClearBlade-UserToken", creds.Token)

	client := http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("getDeviceCount HTTP Error %d: %s", resp.StatusCode, string(body))
	}
	var counts map[string]interface{}
	_ = json.Unmarshal(body, &counts)

	val, ok := counts["counts"].(map[string]interface{})["devices"]
	if ok {
		return int(val.(float64)), nil
	}
	return 0, nil
}

func fetchDevicesFromCSV(service *cbiotcore.ProjectsLocationsRegistriesDevicesService) []*cbiotcore.Device {

	absDevicesCsvFilePath, err := getAbsPath(Args.devicesCsvFile)
	if err != nil {
		log.Fatalln("Cannot resolve devices CSV filepath: ", err.Error())
	}

	if !fileExists(absDevicesCsvFilePath) {
		log.Fatalln("Unable to locate device CSV filepath: ", absDevicesCsvFilePath)
	}

	records := readCsvFile(absDevicesCsvFilePath)
	var deviceIds []string
	for _, line := range records {
		deviceIds = append(deviceIds, line[0])
	}

	fmt.Println()
	spinner := getSpinner("Fetching devices from registry...")

	var devices []*cbiotcore.Device

	if len(deviceIds) > Args.pageSize {
		fmt.Printf("\nMore than %d devices specified in the CSV file. Preparing to batch fetch devices...", Args.pageSize)
		maxIterations := int(math.Floor(float64(len(deviceIds))/float64(Args.pageSize))) + 1
		for i := 0; i < maxIterations; i++ {
			var batchDeviceIds []string
			if i == maxIterations-1 {
				batchDeviceIds = deviceIds[1+i*Args.pageSize:]
			} else if i == 0 {
				batchDeviceIds = deviceIds[i*Args.pageSize : Args.pageSize+i*Args.pageSize]
			} else {
				batchDeviceIds = deviceIds[1+i*Args.pageSize : Args.pageSize+i*Args.pageSize]
			}

			devicesSubset, err := fetchDeviceList(service, batchDeviceIds)
			if err != nil {
				log.Fatalln("Error fetching device list: ", err.Error())
			} else {
				devices = append(devices, devicesSubset...)
			}
			if err := spinner.Add(1); err != nil {
				log.Fatalln("Unable to add to spinner: ", err)
			}
		}
	} else {
		devices, err = fetchDeviceList(service, deviceIds)
		if err != nil {
			log.Fatalln("Error fetching device list: ", err.Error())
		}
	}
	return devices
}

func fetchAllDevices(service *cbiotcore.ProjectsLocationsRegistriesDevicesService) []*cbiotcore.Device {
	var devices []*cbiotcore.Device

	fmt.Println()
	spinner := getSpinner("Fetching all devices from registry...")

	req := service.List(getCBRegistryPath()).PageSize(int64(Args.pageSize))
	resp, err := req.Do()

	if err != nil {
		log.Fatalln("Error fetching all devices: ", err.Error())
	}

	for resp.NextPageToken != "" {
		devices = append(devices, resp.Devices...)

		if err := spinner.Add(1); err != nil {
			log.Fatalln("Unable to add to spinner: ", err)
		}

		resp, err = req.PageToken(resp.NextPageToken).Do()

		if err != nil {
			log.Fatalln("Error fetching all devices: ", err.Error())
			break
		}
	}

	if err == nil {
		devices = append(devices, resp.Devices...)
	}

	return devices
}

func getMissingDeviceIds(devices []*cbiotcore.Device, deviceIds []string) []string {
	missingDeviceIds := make([]string, 0)
	for _, id := range deviceIds {
		found := false
		for _, device := range devices {
			if device.Id == id {
				found = true
			}
		}
		if !found {
			missingDeviceIds = append(missingDeviceIds, id)
		}
	}
	return missingDeviceIds
}

func fetchDeviceList(service *cbiotcore.ProjectsLocationsRegistriesDevicesService, deviceIds []string) ([]*cbiotcore.Device, error) {
	devicesLength := len(deviceIds)

	bar := getProgressBar(devicesLength, "Fetching devices from registry...")

	resp, err := service.List(getCBRegistryPath()).DeviceIds(deviceIds...).Do()

	if err := bar.Finish(); err != nil {
		log.Fatalln("Unable to finish progressbar: ", err)
	}

	if err := bar.Close(); err != nil {
		log.Fatalln("Unable to Close progressbar: ", err)
	}

	if err != nil {
		return nil, err
	} else {
		successMsg := "Fetched " + fmt.Sprint(len(resp.Devices)) + " / " + fmt.Sprint(devicesLength) + " devices!"
		fmt.Println(string(colorGreen), "\n\u2713", successMsg, string(colorReset))

		if len(resp.Devices) != devicesLength {
			fmt.Printf("%sWarning: the following device IDs were not found - %s\n%s", string(colorYellow), strings.Join(getMissingDeviceIds(resp.Devices, deviceIds), ", "), string(colorReset))
		}

		return resp.Devices, nil
	}
}

func migrateDevicesToClearBlade(args *DeviceMigratorArgs, devClient *cb.DevClient, devices []*cbiotcore.Device, errorLogs []ErrorLog) []ErrorLog {
	bar := getProgressBar(len(devices), "Migrating Devices...")
	successfulCreates := 0

	wp := NewWorkerPool(TotalWorkers)
	wp.Run()

	resultC := make(chan ErrorLog, len(devices))

	for i := 0; i < len(devices); i++ {
		idx := i
		if barErr := bar.Add(1); barErr != nil {
			log.Fatalln("Unable to add to progressbar: ", barErr)
		}
		wp.AddTask(func() {
			migrateDevice(resultC, devices[idx])
		})
	}

	for i := 0; i < len(devices); i++ {
		res := <-resultC
		if res.Error != nil {
			errorLogs = append(errorLogs, res)
		} else {
			successfulCreates += 1
		}
	}

	if successfulCreates == len(devices) {
		fmt.Println(string(colorGreen), "\n\n\u2713 Migrated", successfulCreates, "/", len(devices), "devices!", string(colorReset))
	} else {
		fmt.Println(string(colorRed), "\n\n\u2715 Failed to migrate all devices. Migrated", successfulCreates, "/", len(devices), "devices!", string(colorReset))
	}

	return errorLogs
}

func migrateDevice(resultC chan ErrorLog, device *cbiotcore.Device) {
	//* Create or update the device
	err := createOrUpdateDevice(resultC, device)

	// Device Create/Update Successful
	if err == nil && Args.updatePublicKeys && len(device.Credentials) > 0 {
		err = createDeviceCredentials(resultC, device)
		if err != nil {
			return
		}

		//Should roles and permissions be created?
		if Args.createDeviceRole {
			role, err := createRoleForDevice(resultC, device)
			if err != nil {
				return
			}

			var roleId string
			val, ok := role["role_id"]
			if ok {
				roleId = val.(string)
			} else {
				val, ok := role["ID"]
				if ok {
					roleId = val.(string)
				}
			}

			err = addTopicsToRole(resultC, device, roleId)
			if err != nil {
				return
			}

			err = addDeviceToRole(resultC, device)
			if err != nil {
				return
			}
		}
	}

	// Create Device Successful
	if err == nil {
		resultC <- ErrorLog{}
	}
}

func createOrUpdateDevice(resultC chan ErrorLog, device *cbiotcore.Device) error {
	_, err := createDevice(device)

	if err != nil {
		// Checking if device exists - status code 409
		if !strings.Contains(err.Error(), "already exists in system") {
			resultC <- ErrorLog{
				DeviceId: device.Id,
				Context:  "Error when Creating Device",
				Error:    err,
			}
		}

		// If Device exists, patch it
		_, err = updateDevice(device)

		if err != nil {
			resultC <- ErrorLog{
				DeviceId: device.Id,
				Context:  "Error when Patching Device",
				Error:    err,
			}
		}
	}
	return err
}

func updateDevice(device *cbiotcore.Device) (map[string]interface{}, error) {
	return cbDevClient.UpdateDevice(Args.cbSystemKey, device.Id, transform(device, Args.deviceType, Args.columnsCsvFile))
}

func createDevice(device *cbiotcore.Device) (map[string]interface{}, error) {
	return cbDevClient.CreateDevice(Args.cbSystemKey, device.Id, transform(device, Args.deviceType, Args.columnsCsvFile))
}

func createDeviceCredentials(resultC chan ErrorLog, device *cbiotcore.Device) error {
	//Delete the existing device keys
	_, err := deleteDeviceCreds(device.Id)

	if err != nil {
		resultC <- ErrorLog{
			DeviceId: device.Id,
			Context:  "Error when deleting device credentials",
			Error:    err,
		}
		return err
	}

	//Create the device creds
	for _, cred := range device.Credentials {
		_, err = createDeviceCredential(device.Id, cred)

		if err != nil {
			resultC <- ErrorLog{
				DeviceId: device.Id,
				Context:  "Error when creating device credential",
				Error:    err,
			}
			break
		}
	}
	return err
}

func createDeviceCredential(deviceName string, cred *cbiotcore.DeviceCredential) (map[string]interface{}, error) {
	expireTime := ""
	keyFormat := cb.RS256

	if cred.ExpirationTime != "1970-01-01T00:00:00Z" {
		expireTime = cred.ExpirationTime
	}

	switch cred.PublicKey.Format {
	case "RSA_PEM":
		keyFormat = cb.RS256
	case "RSA_X509_PEM":
		keyFormat = cb.RS256_X509
	case "ES256_PEM":
		keyFormat = cb.ES256
	case "ES256_X509_PEM":
		keyFormat = cb.ES256_X509
	default:
		panic("unrecognized escape character")
	}

	return cbDevClient.AddDevicePublicKey(Args.cbSystemKey, deviceName, cred.PublicKey.Key, expireTime, keyFormat)
}

func deleteDeviceCreds(deviceName string) ([]interface{}, error) {
	delQuery := cb.NewQuery()
	delQuery.GreaterThanEqualTo("key_format", 0)

	return cbDevClient.DeleteDevicePublicKey(Args.cbSystemKey, deviceName, delQuery)
}
