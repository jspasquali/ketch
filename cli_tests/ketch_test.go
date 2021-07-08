// +build integration

package cli_tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	ketch   string // ketch executable path
	ingress string // ingress IP

	frameworkCliName  = "myframework"
	frameworkYamlName = "myframework-yaml"
	appImage          = "gcr.io/shipa-ci/sample-go-app:latest"
	appName           = "sample-app"
	cName             = "my-cname.com"
	testEnvvarKey     = "FOO"
	testEnvVarValue   = "BAR"
)

func init() {
	// set ingress IP
	b, err := exec.Command("kubectl", "get", "svc", "traefik", "-o", "jsonpath='{.status.loadBalancer.ingress[0].ip}'").Output()
	if err != nil {
		panic(err)
	}
	ingress = string(b)

	// set ketch executable path
	ketchExecPath := os.Getenv("KETCH_EXECUTABLE_PATH")
	if ketchExecPath != "" {
		ketch = ketchExecPath
		return
	}
	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	ketch = filepath.Join(pwd, "bin", "ketch")
}

// retry tries a command <times> in intervals of <wait> seconds.
// If <match> is never found in command output, an error is returned containing
// all aggregated output.
func retry(cmd *exec.Cmd, match string, times, wait int) error {
	sb := strings.Builder{}
	for i := 0; i < times; i++ {
		b, err := exec.Command(ketch, "app", "info", appName).CombinedOutput()
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteString("\n")

		if strings.Contains(string(b), match) {
			return nil
		}
		if i < times-1 {
			fmt.Println("retrying command: ", cmd.String())
			time.Sleep(time.Second * time.Duration(wait))
		}
	}
	return fmt.Errorf("retry failed on command %s. Output: %s", cmd.String(), sb.String())
}

func TestHelp(t *testing.T) {
	b, err := exec.Command(ketch, "help").CombinedOutput()
	require.Nil(t, err, string(b))
	require.Contains(t, string(b), "For details see https://theketch.io")
	require.Contains(t, string(b), "Available Commands")
	require.Contains(t, string(b), "Flags")
}

func TestFrameworkAddByCLI(t *testing.T) {
	b, err := exec.Command(ketch, "framework", "add", frameworkCliName, "--ingress-service-endpoint", ingress, "--ingress-type", "traefik").CombinedOutput()
	require.Nil(t, err, string(b))
	require.Contains(t, string(b), "Successfully added!")
}

func TestFrameworkList(t *testing.T) {
	b, err := exec.Command(ketch, "framework", "list").CombinedOutput()
	require.Nil(t, err, string(b))
	require.True(t, regexp.MustCompile("NAME[ \t]+STATUS[ \t]+NAMESPACE[ \t]+INGRESS TYPE[ \t]+INGRESS CLASS NAME[ \t]+CLUSTER ISSUER[ \t]+APPS").Match(b), string(b))
	require.True(t, regexp.MustCompile(fmt.Sprintf("%s[ \t]+[Created \t]+ketch-%s[ \t]+traefik[ \t]+traefik", frameworkCliName, frameworkCliName)).Match(b), string(b))
}

func TestFrameworkAddByYaml(t *testing.T) {
	temp, err := os.CreateTemp(t.TempDir(), "*.yaml")
	require.Nil(t, err)
	defer os.Remove(temp.Name())
	temp.WriteString(fmt.Sprintf(`name: %s
app-quota-limit: 1
ingressController:
  className: traefik
  serviceEndpoint: %s
  type: traefik`, frameworkYamlName, ingress))

	b, err := exec.Command(ketch, "framework", "add", temp.Name()).CombinedOutput()
	require.Nil(t, err, string(b))
	require.Contains(t, string(b), "Successfully added!")

	b, err = exec.Command(ketch, "framework", "list").CombinedOutput()
	require.Nil(t, err, string(b))
	require.True(t, regexp.MustCompile(fmt.Sprintf("%s[ \t]+[Created \t]+ketch-%s[ \t]+traefik[ \t]+traefik", frameworkYamlName, frameworkYamlName)).Match(b), string(b))
}

func TestFrameworkUpdateByCli(t *testing.T) {
	b, err := exec.Command(ketch, "framework", "update", frameworkCliName, "--app-quota-limit", "2").CombinedOutput()
	require.Nil(t, err, string(b))
	require.Contains(t, string(b), "Successfully updated!", string(b))
}
func TestFrameworkUpdateByYaml(t *testing.T) {
	temp, err := os.CreateTemp(t.TempDir(), "*.yaml")
	require.Nil(t, err)
	defer os.Remove(temp.Name())
	temp.WriteString(fmt.Sprintf(`name: %s
app-quota-limit: 2
ingressController:
  className: traefik
  serviceEndpoint: %s
  type: traefik`, frameworkYamlName, ingress))
	b, err := exec.Command(ketch, "framework", "update", temp.Name()).CombinedOutput()
	require.Nil(t, err, string(b))
	require.Contains(t, string(b), "Successfully updated!", string(b))
}

func TestFrameworkExport(t *testing.T) {
	b, err := exec.Command(ketch, "framework", "export", frameworkCliName).CombinedOutput()
	require.Nil(t, err, string(b))
	defer os.Remove("framework.yaml")
	b, err = os.ReadFile("framework.yaml")
	require.Nil(t, err, string(b))
	require.Contains(t, string(b), fmt.Sprintf("name: %s", frameworkCliName), string(b))
	require.Contains(t, string(b), fmt.Sprintf("namespace: ketch-%s", frameworkCliName), string(b))
	require.Contains(t, string(b), "appQuotaLimit: 2", string(b))

}

func TestAppDeploy(t *testing.T) {
	b, err := exec.Command(ketch, "app", "deploy", appName, "--framework", frameworkCliName, "-i", appImage).CombinedOutput()
	require.Nil(t, err, string(b))
	require.Equal(t, "", string(b))
}

func TestAppInfo(t *testing.T) {
	cmd := exec.Command(ketch, "app", "info", appName)
	err := retry(cmd, "running", 10, 5)
	require.Nil(t, err)

	b, err := cmd.Output()
	require.Nil(t, err, string(b))
	require.True(t, regexp.MustCompile("DEPLOYMENT VERSION[ \t]+IMAGE[ \t]+PROCESS NAME[ \t]+WEIGHT[ \t]+STATE[ \t]+CMD").Match(b))
	require.True(t, regexp.MustCompile(fmt.Sprintf("1[ \t]+%s[ \t]+web[ \t]+100%%[ \t]+1 running[ \t]", appImage)).Match(b))
}

func TestAppStop(t *testing.T)        {}
func TestAppStart(t *testing.T)       {}
func TestAppLog(t *testing.T)         {}
func TestBuilderList(t *testing.T)    {}
func TestCnameAddRemove(t *testing.T) {}
func TestUnitAdd(t *testing.T)        {}
func TestUnitRemove(t *testing.T)     {}
func TestUnitSet(t *testing.T)        {}
func TestEnvSet(t *testing.T)         {}
func TestEnvGet(t *testing.T)         {}
func TestEnvUnset(t *testing.T)       {}

func TestAppRemove(t *testing.T) {
	b, err := exec.Command(ketch, "app", "remove", appName).CombinedOutput()
	require.Nil(t, err, string(b))
	require.Contains(t, string(b), "Successfully removed!")
}

func TestFrameworkByCliRemove(t *testing.T) {
	b, err := exec.Command(ketch, "framework", "remove", frameworkCliName).CombinedOutput()
	require.Nil(t, err, string(b))
	require.Contains(t, string(b), "Framework successfully removed!")
}

func TestFrameworkByYamlRemove(t *testing.T) {
	b, err := exec.Command(ketch, "framework", "remove", frameworkYamlName).CombinedOutput()
	require.Nil(t, err, string(b))
	require.Contains(t, string(b), "Framework successfully removed!")
}
