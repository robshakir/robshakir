// Package main generates a README.md for a GitHub profile.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"golang.org/x/oauth2"

	log "github.com/golang/glog"
	"github.com/google/go-github/v35/github"
	"github.com/guptarohit/asciigraph"
)

const (
	ghUsername  string = "robshakir"
	fetchEvents int    = 100
)

func pacific(t *time.Time) time.Time {
	pst, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Exitf("LA doesn't exist")
	}
	return t.In(pst)
}

type Events struct {
	Start       time.Time
	Repos       map[string]int
	Actions     map[string]int
	Hours       []float64
	Breadcrumbs []string
}

func NewEvents() *Events {
	return &Events{
		Repos:       map[string]int{},
		Actions:     map[string]int{},
		Hours:       []float64{},
		Breadcrumbs: []string{},
	}
}

func events(ctx context.Context, client *github.Client) (*Events, error) {
	ghevents, _, err := client.Activity.ListEventsPerformedByUser(ctx, ghUsername, false, &github.ListOptions{
		PerPage: fetchEvents,
	})
	if err != nil {
		return nil, err
	}

	e := NewEvents()
	hours := map[int]int{}
	for _, event := range ghevents {
		e.Repos[event.GetRepo().GetName()]++
		e.Actions[event.GetType()]++
		hours[pacific(event.CreatedAt).Hour()]++
	}

	for i := 0; i < 24; i++ {
		e.Hours = append(e.Hours, float64(hours[i]))
	}

	e.Start = pacific(ghevents[fetchEvents-1].CreatedAt)

	return e, nil
}

func plotHourOfDay(events *Events) string {
	outBuf := &bytes.Buffer{}
	as := asciigraph.Plot(events.Hours,
		asciigraph.Width(80),
		asciigraph.Height(20),
		asciigraph.Precision(0),
		asciigraph.Caption("Commits by Hour of Day"),
	)
	outBuf.WriteString(as)

	max := 0.0
	maxHour := 0
	for hour, count := range events.Hours {
		if count > max {
			max = count
			maxHour = hour
		}
	}
	outBuf.WriteString(fmt.Sprintf("\n\nSince %s, I'm most active between %02d:00-%02d:59 - with %.0f events in that hour.\n", events.Start, maxHour, maxHour, max))

	return outBuf.String()
}

func writeActiveRepos(events *Events) string {
	maxEvents := 0
	maxNameLen := 0
	var activeRepo string

	for name, count := range events.Repos {
		if count > maxEvents {
			maxEvents = count
			activeRepo = name
		}
		if l := len(name); l > maxNameLen {
			maxNameLen = l
		}
	}

	div := 1
	if maxEvents > 100 {
		div = 2
	}

	padBuf := &bytes.Buffer{}
	for i := 0; i < maxNameLen+5; i++ {
		padBuf.WriteRune(' ')
	}

	outBuf := &bytes.Buffer{}
	for name, events := range events.Repos {
		barBuf := &bytes.Buffer{}
		for i := 0; i < events/div; i++ {
			barBuf.WriteString("#")
		}
		outBuf.WriteString(padBuf.String())
		outBuf.WriteString(barBuf.String())
		outBuf.WriteString("\n")
		outBuf.WriteString(fmt.Sprintf(" %s", name))
		for i := 0; i < (maxNameLen-len(name))+4; i++ {
			outBuf.WriteRune(' ')
		}
		outBuf.WriteString(barBuf.String())
		outBuf.WriteString("\n")
		outBuf.WriteString(padBuf.String())
		outBuf.WriteString(barBuf.String())
		outBuf.WriteString("\n\n")
	}
	outBuf.WriteString(fmt.Sprintf("\n\nSince %s, I've been most active in %s, with %d events.\n", events.Start, activeRepo, maxEvents))
	return outBuf.String()
}

func main() {
	ctx := context.Background()

	token := os.Getenv("AUTH_TOKEN")
	if token == "" {
		log.Exitf("null token in environment, did you forget to set GITHUB_TOKEN?")
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	events, err := events(context.Background(), client)
	if err != nil {
		log.Exitf("can't read events, %v", err)
	}

	hours := plotHourOfDay(events)
	repos := writeActiveRepos(events)

	outBuf := &bytes.Buffer{}
	outBuf.WriteString("\n```\n")
	outBuf.WriteString(hours)
	outBuf.WriteString("\n```\n")
	outBuf.WriteString("\n\n")
	outBuf.WriteString("\n```\n")
	outBuf.WriteString(repos)
	outBuf.WriteString("\n```\n")

	outBuf.WriteString("[robshakir](mailto:robjs@google.com) is not an official Google product.\n")

	if err := ioutil.WriteFile("README.md", outBuf.Bytes(), 0644); err != nil {
		log.Exitf("can't write file, %v", err)
	}

}
