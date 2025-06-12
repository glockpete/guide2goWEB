package main

import (
	"fmt"
	"sort"
	"strings"
)

func (e *Entry) manageChannels(app *App, sd *SD) (err error) {

	defer func() {
		app.Config.Save()
		Cache.Save()
	}()

	var index, selection int

	var menu Menu
	var entry Entry

	err = Cache.Open()
	if err != nil {
		ShowErr(err)
		return
	}

	Cache.Init()

	menu.Entry = make(map[int]Entry)

	menu.Select = getMsg(0204)
	menu.Headline = e.Value

	// Cancel
	entry.Key = index
	entry.Value = getMsg(0200)
	menu.Entry[index] = entry

	var ch channel

	for _, lineup := range sd.Resp.Status.Lineups {

		index++
		entry.Key = index
		entry.Value = fmt.Sprintf("%s [%s]", lineup.Name, lineup.Lineup)
		entry.Lineup = lineup.Lineup

		menu.Entry[index] = entry

	}

	selection = menu.Show()

	switch selection {

	case 0:
		return

	default:
		entry = menu.Entry[selection]
		ch.Lineup = entry.Lineup

	}

	sd.Req.Parameter = fmt.Sprintf("/%s", entry.Lineup)
	sd.Req.Type = "GET"

	err = sd.Lineups()

	entry.headline()
	var channelNames []string
	var existing string
	var addAll, removeAll bool

	for _, station := range sd.Resp.Lineup.Stations {
		channelNames = append(channelNames, station.Name)
	}

	sort.Strings(channelNames)

	app.Config.GetChannels()

	for _, cName := range channelNames {

		for _, station := range sd.Resp.Lineup.Stations {

			if cName == station.Name {

				var input string

				ch.Name = fmt.Sprintf("%s", station.Name)
				ch.ID = station.StationID

				if ContainsString(app.Config.ChannelIDs, station.StationID) != -1 {
					existing = "+"
				} else {
					existing = "-"
				}

				if !addAll && !removeAll {

					fmt.Println(fmt.Sprintf("[%s] %s [%s] %v", existing, station.Name, station.StationID, station.BroadcastLanguage))

					fmt.Print("(Y) Add Channel, (N) Skip / Remove Channel, (ALL) Add all other Channels, (NONE) Remove all other channels, (SKIP) Skip all Channels: ")
					fmt.Scanln(&input)

					switch strings.ToLower(input) {

					case "y":
						if existing == "-" {
							app.Config.AddChannel(&ch)
						}

					case "n":
						if existing == "+" {
							app.Config.RemoveChannel(&ch)
						}

					case "all":
						app.Config.AddChannel(&ch)
						addAll = true

					case "none":
						app.Config.RemoveChannel(&ch)
						removeAll = true

					case "skip":
						return

					}

				} else {

					if removeAll {
						if existing == "+" {
							app.Config.RemoveChannel(&ch)
						}
					}

					if addAll {
						if existing == "-" {
							app.Config.AddChannel(&ch)
						}
					}

				}

			}

		}

	}

	return
}

func (c *config) AddChannel(ch *channel) {

	c.Station = append(c.Station, *ch)

}

func (c *config) RemoveChannel(ch *channel) {

	var tmp []channel

	for _, old := range c.Station {

		if old.ID != ch.ID {
			tmp = append(tmp, old)
		}

	}

	c.Station = tmp
}

func (c *config) GetChannels() {

	c.ChannelIDs = []string{}

	for _, channel := range c.Station {
		c.ChannelIDs = append(c.ChannelIDs, channel.ID)
	}

}
