package main

type CBConfig struct {
	Project string `json:"project"`
}

type ErrorLog struct {
	Context  string
	Error    error
	DeviceId string
}
