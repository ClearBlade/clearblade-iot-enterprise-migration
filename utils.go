package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	cbiotcore "github.com/clearblade/go-iot"
	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

func fileExists(filename string) bool {
	if _, err := os.Stat(filename); errors.Is(err, os.ErrNotExist) {
		fmt.Println("File path does not exists: ", filename, "Error: ", err)
		return false
	}

	return true
}

func readCsvFile(filePath string) [][]string {
	f, err := os.Open(filePath)
	if err != nil {
		log.Fatalln("Unable to read input file: ", filePath, err)
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		log.Fatalln("Unable to parse file as CSV for: ", filePath, err)
	}

	return records
}

func getCBProjectID(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalln("Error when opening json file: ", err)
	}

	var payload CBConfig
	err = json.Unmarshal(content, &payload)
	if err != nil {
		log.Fatalln("Error during Unmarshal(): ", err)
	}

	return payload.Project
}

func getCBRegistryPath() string {
	val, _ := getAbsPath(Args.cbServiceAccount)
	parent := fmt.Sprintf("projects/%s/locations/%s/registries/%s", getCBProjectID(val), Args.cbRegistryRegion, Args.cbRegistryName)
	return parent
}

func getCBDevicePath(deviceId string) string {
	return fmt.Sprintf("%s/devices/%s", getCBRegistryPath(), deviceId)
}

func readInput(msg string) (string, error) {
	fmt.Print(msg)

	reader := bufio.NewReader(os.Stdin)

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	// remove the delimeter from the string
	input = strings.TrimSuffix(input, "\n")
	input = strings.TrimSuffix(input, "\r")

	return input, nil
}

func readPassword(msg string) (string, error) {
	fmt.Print(msg)

	inputbytes, err := term.ReadPassword(0)
	if err != nil {
		return "", err
	}

	fmt.Println("")

	input := string(inputbytes)

	// remove the delimeter from the string
	input = strings.TrimSuffix(input, "\n")
	input = strings.TrimSuffix(input, "\r")

	return input, nil
}

func getProgressBar(total int, description string) *progressbar.ProgressBar {
	description = string(colorYellow) + description + string(colorReset)
	bar := progressbar.NewOptions(total,
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(30),
		progressbar.OptionSetDescription(description),
		progressbar.OptionShowCount(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	return bar
}

func getSpinner(description string) *progressbar.ProgressBar {
	description = string(colorYellow) + description + string(colorReset)
	bar := progressbar.NewOptions(-1,
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionSetWidth(30),
		progressbar.OptionSetDescription(description),
		progressbar.OptionShowCount(),
	)
	return bar
}

func getAbsPath(path string) (string, error) {
	if len(path) == 0 {
		return path, nil
	}

	if path[0] != '~' {
		return strings.TrimSuffix(path, "\r"), nil
	}

	if len(path) > 1 && path[1] != '/' && path[1] != '\\' {
		return "", errors.New("cannot expand user-specific home dir")
	}

	usr, _ := user.Current()
	dir := usr.HomeDir

	return filepath.Join(dir, path[1:]), nil
}

func transform(device *cbiotcore.Device, deviceType string, csvFile string) map[string]interface{} {
	cbDevice := map[string]interface{}{
		"name":                   device.Id,
		"enabled":                !device.Blocked,
		"type":                   deviceType,
		"allow_key_auth":         false,
		"allow_certificate_auth": true,
	}

	if csvFile != "" {
		absColumnsCsvFilePath, err := getAbsPath(csvFile)
		if err != nil {
			log.Fatalln("Cannot resolve column mapping CSV filepath: ", err.Error())
		}

		if !fileExists(absColumnsCsvFilePath) {
			log.Fatalln("Unable to locate column mapping CSV filepath: ", absColumnsCsvFilePath)
		}

		records := readCsvFile(csvFile)
		for _, line := range records {
			cbDevice[line[1]] = reflect.Indirect(reflect.ValueOf(&device)).FieldByName(line[0])
		}
	}

	return cbDevice
}

func getTimeString(timestamp time.Time) string {
	if timestamp.Unix() == 0 {
		return ""
	}
	return timestamp.Format(time.RFC3339)
}

func generateFailedDevicesCSV(errorLogs []ErrorLog) error {
	currDir, err := os.Getwd()
	if err != nil {
		return err
	}

	failedDevicesFile := fmt.Sprint(currDir, "/failed_devices_", time.Now().Format("2006-01-02T15:04:05"), ".csv")

	if runtime.GOOS == "windows" {
		failedDevicesFile = fmt.Sprint(currDir, "\\failed_devices_", time.Now().Format("2006-01-02T15-04-05"), ".csv")
	}

	f, err := os.OpenFile(failedDevicesFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	defer f.Close()

	fileContents := "context,error,deviceId\n"
	for i := 0; i < len(errorLogs); i++ {
		errMsg := ""
		if errorLogs[i].Error != nil {
			errMsg = errorLogs[i].Error.Error()
		}
		fileContents += fmt.Sprintf(`%s,"%s",%s`, errorLogs[i].Context, errMsg, errorLogs[i].DeviceId)
		fileContents += "\n"
	}

	if _, err := f.WriteString(fileContents); err != nil {
		return err
	}

	return nil
}
