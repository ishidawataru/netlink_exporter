// Copyright (C) 2016 Nippon Telegraph and Telephone Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"syscall"
	"time"

	influx "github.com/influxdata/influxdb/client/v2"
	goplane "github.com/osrg/goplane/netlink"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
)

func main() {
	var host = flag.String("db-host", "localhost:8086", "db host")
	var dbname = flag.String("db-name", "netlink", "db name")
	flag.Parse()

	cli, err := influx.NewHTTPClient(influx.HTTPConfig{
		Addr: fmt.Sprintf("http://%s", *host),
	})
	if err != nil {
		log.Fatal(err)
	}
	_, _, err = cli.Ping(0)
	if err != nil {
		log.Fatal(err)
	}

	q := influx.NewQuery(fmt.Sprintf("CREATE DATABASE %s", *dbname), "", "")
	if res, err := cli.Query(q); err != nil || res.Error() != nil {
		log.Fatal("can not create database ", dbname)
	}

	s, err := nl.Subscribe(syscall.NETLINK_ROUTE, uint(goplane.RTMGRP_NEIGH), uint(goplane.RTMGRP_LINK), uint(goplane.RTMGRP_NOTIFY))
	if err != nil {
		log.Fatal(err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}

	for {
		msgs, err := s.Receive()
		if err != nil {
			log.Fatal(err)
		}

		points := make([]*influx.Point, 0, len(msgs))

		for _, msg := range msgs {
			t := goplane.RTM_TYPE(msg.Header.Type)
			var tags map[string]string
			var fields map[string]interface{}
			var measurement string
			now := time.Now()
			switch t {
			case goplane.RTM_NEWNEIGH, goplane.RTM_DELNEIGH:
				n, err := netlink.NeighDeserialize(msg.Data)
				if err != nil {
					log.Fatal(err)
				}
				link, err := netlink.LinkByIndex(n.LinkIndex)
				if err != nil {
					log.Fatal(err)
				}
				measurement = "neigh"
				tags = map[string]string{
					"host": hostname,
				}
				fields = map[string]interface{}{
					"mac":        n.HardwareAddr.String(),
					"ip":         n.IP.String(),
					"interface":  link.Attrs().Name,
					"family":     goplane.NDA_TYPE(n.Family),
					"state":      goplane.NUD_TYPE(n.State),
					"type":       goplane.RTM_TYPE(n.Type),
					"flags":      goplane.NTF_TYPE(n.Flags),
					"withdrawal": t == goplane.RTM_DELNEIGH,
				}
			case goplane.RTM_NEWLINK, goplane.RTM_DELLINK, goplane.RTM_GETLINK, goplane.RTM_SETLINK:
				l, err := LinkDeserialize(msg.Data)
				if err != nil {
					log.Fatal(err)
				}
				measurement = "link"
				tags = map[string]string{
					"host": hostname,
				}
				fields = map[string]interface{}{
					"name":    l.Attrs().Name,
					"enabled": l.Attrs().Flags&net.FlagUp > 0,
					"running": l.Attrs().Flags&FlagRunning > 0,
				}
			default:
				log.Printf("unhandled event (type: %s)", t)
				continue
			}

			p, err := influx.NewPoint(measurement, tags, fields, now)
			if err != nil {
				log.Fatal(err)
			}
			points = append(points, p)
		}

		if len(points) == 0 {
			continue
		}

		bp, _ := influx.NewBatchPoints(influx.BatchPointsConfig{
			Database:  *dbname,
			Precision: "ms",
		})
		bp.AddPoints(points)
		err = cli.Write(bp)
		if err != nil {
			log.Fatal(err)
		}
	}

}
