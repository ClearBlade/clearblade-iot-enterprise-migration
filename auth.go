package main

import (
	"fmt"
	"log"

	cb "github.com/clearblade/Go-SDK"
)

func authenticateCbEnterprise(Args *DeviceMigratorArgs) (*cb.DevClient, error) {
	devClient := cb.NewDevClientWithAddrs(Args.cbEnterpriseUrl, Args.cbEnterpriseMsgUrl, Args.cbDevEmail, Args.cbDevPwd)
	_, err := devClient.Authenticate()

	if err != nil {
		log.Fatalln("Unable to authenticate ClearBlade developer account: ", err)
	}

	fmt.Println(string(colorGreen), "\n\u2713 ClearBlade Developer Account Authenticated!", string(colorReset))

	return devClient, nil
}
