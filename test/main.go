package main

import (
	"context"
	ha "go-ha-client"
	"net/http"
	"time"
)

func main() {
	client := ha.NewClient(ha.ClientConfig{
		Token: "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJhMDdjYzgwMjEwNTI0NDgzYjkwYjZiM2I2NTk2YzBmMCIsImlhdCI6MTYyODM3Mjk3NCwiZXhwIjoxOTQzNzMyOTc0fQ.HLgUWkF23wnFPERMh8GvyVCaZgSp-1Bo9C0tD7Xi_Rc",
		Host:  "http://192.168.0.165:8123",
		Debug: true,
	}, &http.Client{
		Timeout: 10 * time.Second,
	})

	if err := client.Ping(context.Background()); err != nil {
		panic(err)
	}

	//config, err := client.GetConfig(context.Background())
	//if err != nil {
	//	panic(err)
	//}
	//
	//spew.Dump(config)
	//
	//discoverInfo, err := client.GetDiscoverInfo(context.Background())
	//if err != nil {
	//	panic(err)
	//}
	//
	//spew.Dump(discoverInfo)
	//
	//events, err := client.GetEvents(context.Background())
	//if err != nil {
	//	panic(err)
	//}
	//
	//spew.Dump(events)
	//
	//services, err := client.GetServices(context.Background())
	//if err != nil {
	//	panic(err)
	//}
	//
	//spew.Dump(services)

	//entity := "alarm_control_panel.home_alarm"
	//statusChanges, err := client.GetStateChangesHistory(context.Background(), &ha.StateChangesFilter{
	//	FilterEntityId: entity,
	//	EndTime: time.Now(),
	//})
	//if err != nil {
	//	panic(err)
	//}
	//
	//spew.Dump(statusChanges)

	//logbookRecords, err := client.GetLogbook(context.Background(), &ha.LogbookFilter{
	//	EntityId: "light.2",
	//	EndTime: time.Now(),
	//	StartTime: time.Now().Add(-(8 * time.Hour)),
	//})
	//if err != nil {
	//	panic(err)
	//}
	//
	//spew.Dump(logbookRecords)

	//states, err := client.GetStates(context.Background())
	//if err != nil {
	//	panic(err)
	//}
	//
	//spew.Dump(states)

	//state, err := client.GetStateForEntity(context.Background(),"sensor.ups_input_power_sensitivity")
	//if err != nil {
	//	panic(err)
	//}
	//
	//spew.Dump(state)
	//
	//plainErrorLog, err := client.GetPlainErrorLog(context.Background())
	//if err != nil {
	//	panic(err)
	//}
	//
	//spew.Dump(plainErrorLog)

	//camImg, err := client.GetCameraJpeg(context.Background(), "camera.octoprint")
	//if err != nil {
	//	panic(err)
	//}
	//
	//f, err := os.Create("camera.jpg")
	//if err != nil {
	//	panic(err)
	//}
	//defer f.Close()
	//if err := jpeg.Encode(f, camImg, nil); err != nil {
	//	panic(err)
	//}

	if _, err := client.CallService(context.Background(), ha.NewTurnLightOnCmd("light.extended_color_light_4")); err != nil {
		panic(err)
	}

	time.Sleep(10 * time.Second)

	if _, err := client.CallService(context.Background(), ha.NewTurnLightOffCmd("light.extended_color_light_4")); err != nil {
		panic(err)
	}
}
