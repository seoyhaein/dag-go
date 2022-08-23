package podbridge

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/containers/image/v5/manifest"
	"github.com/pkg/errors"
)

/*
The core of the healthcheck is the command.
Podman will execute the command inside the target container and wait for either a "0" or "failure  exit" code. For example, if you have a container that runs an Nginx server, your healthcheck command could be something as simple as a curl command successfully connecting to the web port to make sure Nginx is responsive.

The other four components are related to the scheduling of the healthcheck itself.
They are optional and have defaults should you choose to not specify values for each one.
Retries defines the number of consecutive failed healthchecks that need to occur before the container is marked as “unhealthy.” A successful healthcheck resets the retry counter.

The interval metric describes the time between running the healthcheck command.
Determining the interval value is a bit of an art.
Make it too small and your system will spend a lot of time running healthchecks; make the interval too large and you struggle with catching time outs.
The value is a time duration like “30s” or “1h2m.”

Note: A duration string is a possibly signed sequence of decimal numbers, each with optional fraction and a unit suffix, such as "300ms," "-1.5h," or "2h45m." Valid time units are "ns," "us," (or "µs"), "ms," "s," "m," and "h."

The fourth component is the start-period.
This describes the time between when the container starts and when you want to ignore healthcheck failures.
Put more simply, if a healthcheck fails during this time, it will not count as a failure.
During this time, the container’s healthcheck status will be starting.
If a healthcheck returns successfully, the container’s healthcheck status will change from starting to healthy.

The last component is the timeout definition.
Like the interval value, it is a time duration.
It describes the period of time the healthcheck itself must complete before being considered unsuccessful.
*/

type HealthCheck struct {
	InCmd       string
	Interval    string
	Retries     uint
	Timeout     string
	StartPeriod string
}

func (h *HealthCheck) SetHealthChecker() (*manifest.Schema2HealthConfig, error) {
	healthConfig, err := makeHealthCheckFromCli(h.InCmd, h.Interval, h.Retries, h.Timeout, h.StartPeriod)
	if err != nil {
		return nil, err
	}
	return healthConfig, nil
}

func CreateHealthCheck(inCmd, interval string, retries uint, timeout, startPeriod string) *HealthCheck {
	return &HealthCheck{
		InCmd:       inCmd,
		Interval:    interval,
		Retries:     retries,
		Timeout:     timeout,
		StartPeriod: startPeriod,
	}
}

func SetHealthChecker(inCmd, interval string, retries uint, timeout, startPeriod string) (*manifest.Schema2HealthConfig, error) {
	healthConfig, err := makeHealthCheckFromCli(inCmd, interval, retries, timeout, startPeriod)
	if err != nil {
		return nil, err
	}
	return healthConfig, nil
}

// TODO 수정하자.
func makeHealthCheckFromCli(inCmd, interval string, retries uint, timeout, startPeriod string) (*manifest.Schema2HealthConfig, error) {
	cmdArr := []string{}
	isArr := true
	err := json.Unmarshal([]byte(inCmd), &cmdArr) // array unmarshalling
	if err != nil {
		cmdArr = strings.SplitN(inCmd, " ", 2) // default for compat
		isArr = false
	}
	// Every healthcheck requires a command
	if len(cmdArr) == 0 {
		return nil, errors.New("Must define a healthcheck command for all healthchecks")
	}

	var concat string
	if cmdArr[0] == "CMD" || cmdArr[0] == "none" { // this is for compat, we are already split properly for most compat cases
		cmdArr = strings.Fields(inCmd)
	} else if cmdArr[0] != "CMD-SHELL" { // this is for podman side of things, won't contain the keywords
		if isArr && len(cmdArr) > 1 { // an array of consecutive commands
			cmdArr = append([]string{"CMD"}, cmdArr...)
		} else { // one singular command
			if len(cmdArr) == 1 {
				concat = cmdArr[0]
			} else {
				concat = strings.Join(cmdArr[0:], " ")
			}
			cmdArr = append([]string{"CMD-SHELL"}, concat)
		}
	}

	if cmdArr[0] == "none" { // if specified to remove healtcheck
		cmdArr = []string{"NONE"}
	}

	// healthcheck is by default an array, so we simply pass the user input
	hc := manifest.Schema2HealthConfig{
		Test: cmdArr,
	}

	if interval == "disable" {
		interval = "0"
	}
	intervalDuration, err := time.ParseDuration(interval)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid healthcheck-interval")
	}

	hc.Interval = intervalDuration

	if retries < 1 {
		return nil, errors.New("healthcheck-retries must be greater than 0")
	}
	hc.Retries = int(retries)
	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid healthcheck-timeout")
	}
	if timeoutDuration < time.Duration(1) {
		return nil, errors.New("healthcheck-timeout must be at least 1 second")
	}
	hc.Timeout = timeoutDuration

	startPeriodDuration, err := time.ParseDuration(startPeriod)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid healthcheck-start-period")
	}
	if startPeriodDuration < time.Duration(0) {
		return nil, errors.New("healthcheck-start-period must be 0 seconds or greater")
	}
	hc.StartPeriod = startPeriodDuration

	return &hc, nil
}
