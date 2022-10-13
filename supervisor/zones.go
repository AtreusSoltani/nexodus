package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/redhat-et/jaywalking/supervisor/ipam"
	log "github.com/sirupsen/logrus"
)

type MsgTypes struct {
	ID    string
	Event string
	Zone  string
	Peer  Peer
}

func (sup *Supervisor) AddPeer(ctx context.Context, msgEvent MsgEvent) error {

	var nodeZone string
	var ipamPrefix string
	var err error
	z := Zone{}
	for _, zone := range sup.Zones {
		if msgEvent.Peer.Zone == zone.Name {
			nodeZone = msgEvent.Peer.Zone
			ipamPrefix = zone.IpCidr
			z = zone
		}
	}
	if nodeZone == "" {
		return fmt.Errorf("requested zone [ %s ] was not found, has it been created yet?\n", msgEvent.Peer.Zone)
	}

	peer := Peer{}
	var ip string
	// If this was a static address request
	// TODO: handle a user requesting an IP not in the IPAM prefix
	if msgEvent.Peer.NodeAddress != "" {
		if err := ipam.ValidateIp(msgEvent.Peer.NodeAddress); err == nil {
			ip, err = z.ZoneIpam.RequestSpecificIP(ctx, msgEvent.Peer.NodeAddress, ipamPrefix)
			if err != nil {
				log.Errorf("failed to assign the requested address %s, assigning an address from the pool %v\n", msgEvent.Peer.NodeAddress, err)
				ip, err = z.ZoneIpam.RequestIP(ctx, ipamPrefix)
				if err != nil {
					log.Errorf("failed to acquire an IPAM assigned address %v\n", err)
				}
			}
		}
	} else {
		ip, err = z.ZoneIpam.RequestIP(ctx, ipamPrefix)
		if err != nil {
			log.Errorf("failed to acquire an IPAM assigned address %v\n", err)
		}
	}
	// allocate a child prefix if requested
	var childPrefix string
	if msgEvent.Peer.ChildPrefix != "" {
		childPrefix, err = z.ZoneIpam.RequestChildPrefix(ctx, msgEvent.Peer.ChildPrefix)
		if err != nil {
			log.Errorf("%v\n", err)
		}
	}
	// save the ipam to persistent storage
	z.ZoneIpam.IpamSave(ctx)

	// construct the new node
	peer = msgEvent.newNode(ip, childPrefix)
	log.Debugf("node allocated: %+v\n", peer)

	for i, zone := range sup.Zones {
		if zone.Name == nodeZone {
			if sup.Zones[i].NodeMap == nil {
				sup.Zones[i].NodeMap = make(map[string]Peer)
			}
			// delete the old k/v pair if one exists and replace it with the new registration data
			if _, ok := sup.Zones[i].NodeMap[msgEvent.Peer.PublicKey]; ok {
				delete(sup.Zones[i].NodeMap, msgEvent.Peer.PublicKey)
			}
			sup.Zones[i].NodeMap[msgEvent.Peer.PublicKey] = peer
		}
	}

	return nil
}

func (sup *Supervisor) MessageHandling(ctx context.Context) {

	pub := NewPubsub(NewRedisClient(sup.streamSocket, sup.streamPass))
	sub := NewPubsub(NewRedisClient(sup.streamSocket, sup.streamPass))

	// channel for async messages from the zone subscription
	controllerChan := make(chan string)

	go func() {
		sub.subscribe(ctx, zoneChannelController, controllerChan)
		log.Debugf("Listening on channel: %s", zoneChannelController)

		for {
			msg := <-controllerChan
			msgEvent := msgHandler(msg)
			switch msgEvent.Event {
			// TODO implement error chans
			case registerNodeRequest:
				log.Debugf("Register node msg received on channel [ %s ]\n", zoneChannelController)
				log.Debugf("Recieved registration request: %+v\n", msgEvent.Peer)
				if msgEvent.Peer.PublicKey != "" {
					err := sup.AddPeer(ctx, msgEvent)
					// append all peers into the updated peer list to be published
					if err == nil {
						var peerList []Peer
						for _, zone := range sup.Zones {
							if zone.Name == msgEvent.Peer.Zone {
								for pubKey, nodeElements := range zone.NodeMap {
									log.Printf("NodeState - PublicKey: [%s] EndpointIP [%s] AllowedIPs [%s] NodeAddress [%s] Zone [%s] ChildPrefix [%s]\n",
										pubKey, nodeElements.EndpointIP, nodeElements.AllowedIPs, nodeElements.NodeAddress, nodeElements.Zone, nodeElements.ChildPrefix)
									// append the new node to the updated peer listing
									peerList = append(peerList, nodeElements)
								}
							}
							// publishPeers the latest peer list
							pub.publishPeers(ctx, zoneChannelController, peerList)
						}
					} else {
						log.Errorf("Peer was not added: %v", err)
						// TODO: return an error to the agent on a message chan
					}
				}
			}
		}
	}()
}

// handleMsg deals with streaming messages
func msgHandler(payload string) MsgEvent {
	var peer MsgEvent
	err := json.Unmarshal([]byte(payload), &peer)
	if err != nil {
		log.Debugf("HandleMsg unmarshall error: %v\n", err)
		return peer
	}
	return peer
}

func NewPubMessage(data string) (string, string) {
	id := uuid.NewString()
	msg, _ := json.Marshal(&MsgTypes{
		ID:    id,
		Event: data,
	})
	return id, string(msg)
}

// TODO example: do we want to implement a UUID with channel messages?
func PubMessage(ctx context.Context, channel, data string) {
	id, msg := NewPubMessage(data)
	err := redisDB.Publish(ctx, channel, msg).Err()
	if err != nil {
		log.Errorf("Sending msg ID %s failed, %v\n", id, err)
		return
	}
	log.Printf("Sent Message: %s\n", msg)
}