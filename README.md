# go-ha-client
Go client for home-assistant REST API. Tested with home-assistant `core-2021.7.2`


### Basic usage
Change `Token` and `Host` to your actual home-assistant bearer token and address
Check home-assistant documentation how get access token.

```go
package main

import (
	"context"
	"fmt"
	ha "github.com/mkelcik/go-ha-client"
	"net/http"
	"time"
)

func main() {
	client := ha.NewClient(ha.ClientConfig{Token: "mytoken", Host: "http://my-ha.home"}, &http.Client{
		Timeout: 30 * time.Second,
	})
    
	// ping instance
	if err := client.Ping(context.Background()); err != nil {
		fmt.Println("connection error", err)
	} else {
		fmt.Println("connection ok")
	}

	// example of home-assistant instance info
	discoverInfo, err := client.GetDiscoverInfo(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v", discoverInfo)
}
```

### Examples

To turn light with entity id `light.light_1` on, we can use `NewTurnLightOnCmd` helper, to create command and call service.
```go
// turn light on
if _, err := client.CallService(context.Background(), ha.NewTurnLightOnCmd("light.light_1")); err != nil {
	panic(err)
}

// turn light off 
if _, err := client.CallService(context.Background(), ha.NewTurnLightOffCmd("light.light_1")); err != nil {
	panic(err)
}
```
or turn `switch.switch_1` off without helper
```go
if _, err := client.CallService(context.Background(), DefaultServiceCmd{
    Service:  "turn_off",
    Domain:   "switch", 
    EntityId: "switch.switch_1",
}); err != nil {
	panic(err)
}
```

Take and save picture from camera 
```go
camImg, err := client.GetCameraJpeg(context.Background(), "camera.my_camera")
if err != nil {
	panic(err)
}

f, err := os.Create("camera.jpg")
if err != nil {
	panic(err)
}
defer f.Close()

if err := jpeg.Encode(f, camImg, nil); err != nil {
	panic(err)
}
```