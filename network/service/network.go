package service

import (
	"BCDns_0.1/bcDns/conf"
	"BCDns_0.1/certificateAuthority/service"
	"encoding/json"
	"fmt"
	"github.com/HJXSaber/memberlist"
	"log"
)

type DnsNet struct {
	Network    *memberlist.Memberlist
	broadCasts *memberlist.TransmitLimitedQueue
}

type MessageTypeT uint8

const (
	Proposal MessageTypeT = iota + 1
	AuditResponse
	ViewChange
	ViewChangeResult
	RetrieveLeader
	RetrieveLeaderResponse
	Commit
	Block
	ProposalResult
)

type Massage struct {
	MessageType MessageTypeT
	Payload     []byte
}

var (
	AuditResponseChan          chan []byte
	ProposalChan               chan []byte
	ViewChangeMsgChan          chan []byte
	ViewChangeResultChan       chan []byte
	RetrieveLeaderMsgChan      chan []byte
	RetrieveLeaderResponseChan chan []byte
	CommitChan                 chan []byte
	BlockChan                  chan []byte
	ProposalResultChan         chan []byte
)

//Can not broadcast msg whose size is longer than 1350B
//When the size of msg is longer than 1350B. We have to transfer it by reliable channel
func (net DnsNet) BroadcastMsg(jsonData []byte, t MessageTypeT) {
	msg := Massage{
		MessageType: t,
		Payload:     jsonData,
	}
	msgByte, err := json.Marshal(msg)
	if err != nil {
		fmt.Printf("[BroadcastMsg] json.Marshal failed err=%v\n", err)
		return
	}
	if len(msgByte) >= 1350 {
		//TODO
		for _, node := range net.Network.Members() {
			err := net.Network.SendReliable(node, msgByte)
			if err != nil {
				fmt.Println("Broadcast msg failed", err)
				continue
			}
		}
	} else {
		net.broadCasts.QueueBroadcast(&Broadcast{
			Msg:    msgByte,
			Notify: nil,
		})
	}
}

func (net DnsNet) SendTo(jsonData []byte, t MessageTypeT, to int) {
	msg := Massage{
		MessageType: t,
		Payload:     jsonData,
	}
	msgByte, err := json.Marshal(msg)
	if err != nil {
		fmt.Printf("[SendTo] json.Marshal failed err=%v\n", err)
		return
	}

	err = net.Network.SendReliable(
		service.CertificateAuthorityX509.CertificatesOrder[to].Member.(*memberlist.Node), msgByte)
	if err != nil {
		fmt.Println("[SendTo] msg failed", err)
	}
}

func (net DnsNet) SendToLeader(jsonData []byte, t MessageTypeT) {
	msg := Massage{
		MessageType: t,
		Payload:     jsonData,
	}
	msgByte, err := json.Marshal(msg)
	if err != nil {
		fmt.Printf("[SendToLeader] json.Marshal failed err=%v\n", err)
		return
	}
	err = net.Network.SendReliable(
		service.CertificateAuthorityX509.CertificatesOrder[Leader.LeaderId].Member.(*memberlist.Node), msgByte)
	if err != nil {
		fmt.Println("[SendToLeader] msg failed", err)
	}
}

var (
	P2PNet DnsNet
)

func init() {
	config := memberlist.DefaultLANConfig()
	config.BindPort = conf.BCDnsConfig.Port
	config.Delegate = &Delegate{}
	config.Name = conf.BCDnsConfig.HostName

	var err error
	P2PNet.Network, err = memberlist.Create(config)
	if err != nil {
		//TODO
		log.Fatal("Initial network failed", err)
	}

	seeds := service.CertificateAuthorityX509.GetSeeds()
	_, err = P2PNet.Network.Join(seeds)
	for _, member := range P2PNet.Network.Members() {
		for i, cert := range service.CertificateAuthorityX509.CertificatesOrder {
			if cert.Cert.IPAddresses[0].Equal(member.Addr) {
				service.CertificateAuthorityX509.CertificatesOrder[i].Member = &member
			}
		}
	}
	if err != nil {
		//TODO
		log.Fatal("Join failed ", err)
	}

	P2PNet.broadCasts = &memberlist.TransmitLimitedQueue{
		NumNodes: func() int {
			return P2PNet.Network.NumMembers()
		},
		RetransmitMult: 3,
	}
	AuditResponseChan = make(chan []byte, 1024)
	ProposalChan = make(chan []byte, 1024)
	ViewChangeMsgChan = make(chan []byte, 1024)
	ViewChangeResultChan = make(chan []byte, 1024)
	RetrieveLeaderMsgChan = make(chan []byte, 1024)
	RetrieveLeaderResponseChan = make(chan []byte, 1024)
	CommitChan = make(chan []byte, 1024)
	BlockChan = make(chan []byte, 1024)
	ProposalResultChan = make(chan []byte, 1024)
}

type Broadcast struct {
	Msg    []byte
	Notify chan<- struct{}
}

func (b *Broadcast) Finished() {
	if b.Notify != nil {
		close(b.Notify)
	}
}

func (*Broadcast) Invalidates(b memberlist.Broadcast) bool {
	return false
}

func (b *Broadcast) Message() []byte {
	return b.Msg
}

type Delegate struct{}

func (*Delegate) NodeMeta(limit int) []byte {
	return []byte{}
}

func (*Delegate) NotifyMsg(data []byte) {
	var msg Massage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		fmt.Printf("[NotifyMsg] json.Marshal failed err=%v\n", err)
		return
	}
	switch msg.MessageType {
	case Proposal:
		ProposalChan <- msg.Payload
	case AuditResponse:
		AuditResponseChan <- msg.Payload
	case ViewChange:
		ViewChangeMsgChan <- msg.Payload
	case ViewChangeResult:
		ViewChangeResultChan <- msg.Payload
	case RetrieveLeader:
		RetrieveLeaderMsgChan <- msg.Payload
	case RetrieveLeaderResponse:
		RetrieveLeaderResponseChan <- msg.Payload
	case Commit:
		CommitChan <- msg.Payload
	case Block:
		BlockChan <- msg.Payload
	case ProposalResult:
		ProposalResultChan <- msg.Payload
	}

}

func (*Delegate) GetBroadcasts(overhead, limit int) [][]byte {
	return P2PNet.broadCasts.GetBroadcasts(overhead, limit)
}

//exchange local data with remote peer. certificate verify through this func
func (*Delegate) LocalState(join bool) []byte {
	_, certBytes := service.CertificateAuthorityX509.GetLocalCertificate()
	if certBytes == nil {
		return nil
	}
	return certBytes
}

func (*Delegate) MergeRemoteState(buf []byte, join bool) {
	if !join {
		fmt.Println("MergeState TODO")
	}
}

func (*Delegate) ValidateCert(buf []byte) bool {
	return service.CertificateAuthorityX509.VerifyCertificate(buf)
}