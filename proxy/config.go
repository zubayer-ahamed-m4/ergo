package proxy

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"sync"
	"time"
)

//Service holds the details of the service (Name and URL)
type Service struct {
	Name string
	URL  string
}

//NewService gets the new service.
func NewService(name, url string) Service {
	return Service{
		Name: name,
		URL:  url,
	}
}

// Empty service means no name or no url
func (s Service) Empty() bool {
	return s.Name == "" || s.URL == ""
}

//Config holds the configuration for the proxy.
type Config struct {
	mutex      sync.Mutex
	lastChange time.Time
	size       int64

	Port       string
	Domain     string
	URLPattern string
	Verbose    bool
	Services   map[string]Service
	ConfigFile string
}

//NewConfig gets the new config.
func NewConfig() *Config {
	return &Config{
		Port:       "2000",
		Domain:     ".dev",
		URLPattern: `.*\.dev$`,
		Services:   make(map[string]Service),
	}
}

var once sync.Once
var domainPattern *regexp.Regexp

//GetService gets the service for the given host.
func (c *Config) GetService(host string) Service {
	once.Do(func() {
		domainPattern = regexp.MustCompile(`((\w*\:\/\/)?.+)(` + c.Domain + `)`)
	})

	parts := domainPattern.FindAllStringSubmatch(host, -1)

	if len(parts) < 1 {
		return Service{}
	}

	return c.Services[parts[0][1]]
}

//GetProxyPacURL returns the correct url for the pac file
func (c *Config) GetProxyPacURL() string {
	return "http://127.0.0.1:" + c.Port + "/proxy.pac"
}

//AddService add a service using the correct key
func (c *Config) AddService(service Service) error {
	if service.Empty() {
		return fmt.Errorf("Service is invalid")
	}

	c.Services[service.Name] = service
	return nil
}

//LoadServices loads the services from filepath, returns an error
//if the configuration could not be parsed
func (c *Config) LoadServices() error {
	services, err := readServicesFromFile(c.ConfigFile)
	if err != nil {
		return err
	}

	for _, s := range services {
		if !s.Empty() {
			c.mutex.Lock()
			{
				c.AddService(s)
			}
			c.mutex.Unlock()
		}
	}

	return nil
}

// WatchConfigFile listen for file changes and updates the config services
func (c *Config) WatchConfigFile(tickerChan <-chan time.Time) {
	for _ = range tickerChan {
		info, err := os.Stat(c.ConfigFile)
		if err != nil {
			log.Printf("Error reading config file: %s\r\n", err.Error())
			continue
		}

		if info.ModTime().Before(c.lastChange) || info.Size() != c.size {
			c.size = info.Size()
			c.lastChange = info.ModTime()

			err = c.LoadServices()
			if err != nil {
				log.Printf("Error reading the modified config file")
				continue
			}
		}
	}
}

// readServicesFromFile reads the given path and parse it into services
func readServicesFromFile(filepath string) ([]Service, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("file error: %v", err)
	}
	defer file.Close()

	services := []Service{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		declaration := regexp.MustCompile(`(\S+)`)
		config := declaration.FindAllString(line, -1)
		if config == nil {
			continue
		}
		if len(config) != 2 {
			return nil, fmt.Errorf("file error: invalid format `%v` expected `{NAME} {URL}`", line)
		}
		name, url := config[0], config[1]
		services = append(services, Service{Name: name, URL: url})
	}

	return services, nil
}

//AddService adds new service to the filepath
func AddService(filepath string, service Service) error {
	file, e := os.OpenFile(filepath, os.O_APPEND|os.O_WRONLY, os.ModeAppend)

	if e != nil {
		return fmt.Errorf("File error: %v", e)
	}

	defer file.Close()

	serviceStr := service.Name + " " + service.URL + "\n"
	_, err := file.WriteString(serviceStr)
	return err
}

// RemoveService removes a service from the filepath
func RemoveService(filepath string, service Service) error {
	file, err := ioutil.ReadFile(filepath)
	if err != nil {
		log.Printf("File error: %v\n", err)
		return err
	}

	serviceRegex := regexp.MustCompile(service.Name + "\\s+" + service.URL + "\n")

	file = serviceRegex.ReplaceAll(file, []byte("\n"))

	ioutil.WriteFile(filepath, file, 0755)

	return nil
}
