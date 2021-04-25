package config

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"

	"github.com/lateralusd/lateralus/email"
	log "github.com/sirupsen/logrus"

	"github.com/lateralusd/lateralus/util"
)

// TemplateFields keep fields that will be used in templates
type TemplateFields struct {
	Name         string `json:"templateName"`
	AttackerName string `json:"attackerName"`
	URL          string `json:"url"`
	Custom       string `json:"custom"`
}

// User struct gets populated from .csv file. These are the targets
type User struct {
	Name  string
	Email string
	URL   string
}

// Options is the main configuration structure
type Options struct {
	SingleURL      *bool   `json:"singleUrl"`
	ConfigFile     *string `json:"config"`
	TemplateName   *string `json:"template"`
	TargetsFile    *string `json:"targets"`
	Generate       *bool   `json:"generateUrl"`
	GenerateLength *int    `json:"generateLength"`
	SMTPConfig     *string `json:"smtpconfig"`
	Subject        *string `json:"subject"`
	From           *string `json:"from"`
	ReportName     *string `json:"report"`
	Delay          *int    `json:"delay"`
	Parse          *string `json:"parseMdl"`
	Priority       *string `json:"priority"`
	Signature      *string `json:"signature"`
	StartTime      string
	EndTime        string
	Targets        []User
	*TemplateFields
}

var (
	options = Options{
		SingleURL:      flag.Bool("singleUrl", true, "Use the same URL for all targets"),
		ConfigFile:     flag.String("config", "", "Config file to read parameters from"),
		TemplateName:   flag.String("template", "", "Email template from templates/ directory"),
		TargetsFile:    flag.String("targets", "", "File consisting of targets data (name, lastname, email, url)"),
		Generate:       flag.Bool("generate", false, "If set to true, parameter url needs to have <CHANGE> part"),
		GenerateLength: flag.Int("generateLength", 8, "Length of variable part of url with maximum of 36"),
		SMTPConfig:     flag.String("smtpConfig", "conf/smtp.conf", "SMTP config file"),
		Subject:        flag.String("subject", "Mail Subject", "Subject that will be used for emails"),
		From:           flag.String("from", "", "From field for an email. If not provided, will be the same as attackerName"),
		ReportName:     flag.String("report", "", "Report name"),
		Delay:          flag.Int("delay", 0, "delay between sending mails in seconds"),
		Parse:          flag.String("parseMdl", "", "Path to Modlishka control db file"),
		Priority:       flag.String("priority", "low", "priority to send email, can be low or high"),
		Signature:      flag.String("signature", "", "path to signature .html file"),
	}
	s        = TemplateFields{}
	csvLines [][]string
)

// SMTPServer is object that will be used for sending mails (Client)
var SMTPServer *email.SMTP

// ParseConfiguration is the main function that will be called from main binary to initialize all flags and parse all config files.
func ParseConfiguration(ctime string) (*Options, error) {
	SMTPServer = &email.SMTP{}

	flag.StringVar(&s.Name, "templateName", "", "Email template name")
	flag.StringVar(&s.AttackerName, "attackerName", "", "Attacker name to use in template")
	flag.StringVar(&s.URL, "url", "", "Single url to include in emails")
	flag.StringVar(&s.Custom, "custom", "", "Custom words to include in template")

	flag.Parse()

	options.TemplateFields = &s

	options.StartTime = ctime

	// If JSON config file is in use
	if *options.ConfigFile != "" {
		options.parseJSON(*options.ConfigFile)
	}

	if *options.Parse != "" {
		util.ParseModlishka(*options.Parse)
		os.Exit(1)
	}

	// Parse targets from csv file
	parseCSV(*options.TargetsFile)

	log.Infof("Read %d targets from %s\n", len(options.Targets), *options.TargetsFile)

	// Fill user URL field in case of single field

	// Url param is passed, we have to do something with it
	if options.TemplateFields.URL != "" {
		// Fill every user url with the same field
		if *options.SingleURL {
			for i := range options.Targets {
				options.Targets[i].URL = options.TemplateFields.URL
			}
		} else { // Substitute <CHANGE> part of url with UUID of *options.GenerateLength length
			if strings.Contains(options.TemplateFields.URL, "<CHANGE>") {
				url := options.TemplateFields.URL
				for i := range options.Targets {
					userURL := url[:strings.Index(url, "<CHANGE>")] + util.GenerateUUID(*options.GenerateLength)
					options.Targets[i].URL = userURL
				}
			}
		}

	}

	// Parse smtp configuration
	options.parseSMTP()

	return &options, nil
}

/*
ParseTemplate is method that for each target creates an email body.
First parameter it returns are slice of targets emails.
Second parameter are slices of email bodies for each user.
*/
func (c *Options) ParseTemplate() ([]string, []string, []string, error) {
	t, err := template.ParseFiles(*c.TemplateName)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ParseTemplate: %v", err)
	}

	var names, to, bodies []string

	for _, user := range c.Targets {
		var buf bytes.Buffer
		tData := TemplateFields{
			Name:         user.Name,
			AttackerName: c.TemplateFields.AttackerName,
			URL:          user.URL,
			Custom:       c.TemplateFields.Custom,
		}
		_ = t.Execute(&buf, tData)
		names = append(names, user.Name)
		to = append(to, user.Email)
		bodies = append(bodies, buf.String())
	}

	return names, to, bodies, nil
}

func (c *Options) parseSMTP() error {
	if len(*c.SMTPConfig) > 1 {
		file, err := os.Open(*options.SMTPConfig)
		defer file.Close()
		data, _ := ioutil.ReadAll(file)
		if err != nil {
			return fmt.Errorf("parseSMTP: %v", err)
		}
		err = json.Unmarshal(data, SMTPServer)
		if err != nil {
			return fmt.Errorf("parseSMTP: %v", err)
		}
	}
	SMTPServer.Priority = *c.Priority
	SMTPServer.Signature = *c.Signature

	return nil
}

func (c *Options) parseJSON(file string) error {
	ct, err := os.Open(file)
	defer ct.Close()
	if err != nil {
		return fmt.Errorf("parseJSON: %v", err)
	}

	ctb, _ := ioutil.ReadAll(ct)
	err = json.Unmarshal(ctb, &c)
	if err != nil {
		return fmt.Errorf("parseJSON: %v", err)
	}

	err = json.Unmarshal(ctb, &s)
	if err != nil {
		return fmt.Errorf("parseJSON: %v", err)
	}

	options.TemplateFields = &s

	return nil
}

func parseCSV(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("parseCSV: %v", err)
	}

	csvLines, err = csv.NewReader(f).ReadAll()
	if err != nil {
		return fmt.Errorf("parseCSV: %v", err)
	}

	for _, line := range csvLines {
		options.Targets = append(options.Targets, User{Name: line[0], Email: line[1], URL: ""})
	}

	return nil
}
