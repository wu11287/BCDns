package service

import (
	"BCDns_0.1/bcDns/conf"
	"BCDns_0.1/certificateAuthority/service"
	"BCDns_0.1/messages"
	service2 "BCDns_0.1/network/service"
	"context"
	"encoding/json"
	"fmt"
	"github.com/op/go-logging"
	"net"
	"sync"
	"time"
)

var (
	logger     *logging.Logger // package-level logger
	UdpAddress = "127.0.0.1:8888"
)

type Proposer struct {
	Mutex sync.Mutex

	Proposals map[string]messages.ProposalMessage
	Replys    map[string]map[string]uint8
	Contexts  map[string]context.CancelFunc
	Conn      *net.UDPConn
	OrderChan chan []byte
}

func NewProposer() *Proposer {
	udpaddr, err := net.ResolveUDPAddr("udp", UdpAddress)
	if err != nil {
		panic(err)
	}
	conn, err := net.ListenUDP("udp", udpaddr)
	if err != nil {
		panic(err)
	}
	return &Proposer{
		Proposals: map[string]messages.ProposalMessage{},
		Replys:    map[string]map[string]uint8{},
		Contexts:  map[string]context.CancelFunc{},
		OrderChan: make(chan []byte, 1024),
		Conn:      conn,
	}
}

func init() {
	logger = logging.MustGetLogger("consensusMy")
}

func (p *Proposer) Run(done chan uint) {
	var (
		err error
	)
	defer close(done)
	go p.ReceiveOrder()
	count := 0
	for {
		select {
		case msgByte := <-p.OrderChan:
			var msg Order
			err = json.Unmarshal(msgByte, &msg)
			if err != nil {
				continue
			}
			p.handleOrder(msg)
		case msgByte := <-service2.ProposalReplyChan:
			var msg messages.ProposalReplyMessage
			err := json.Unmarshal(msgByte, &msg)
			if err != nil {
				logger.Warningf("[Proposer.Run] json.Unmarshal error=%v", err)
				continue
			}
			if !msg.VerifySignature() {
				logger.Warningf("[Proposer.Run] Signature is invalid")
				continue
			}
			p.Mutex.Lock()
			if _, ok := p.Proposals[string(msg.Id)]; ok {
				p.Replys[string(msg.Id)][msg.From] = 0
				if service.CertificateAuthorityX509.Check(len(p.Replys[string(msg.Id)])) {
					fmt.Printf("[Proposer.Run] ProposalMsgT execute successfully %v\n", p.Proposals[string(msg.Id)])
					fmt.Println("count", count)
					count++
					delete(p.Proposals, string(msg.Id))
					delete(p.Replys, string(msg.Id))
					p.Contexts[string(msg.Id)]()
					delete(p.Contexts, string(msg.Id))
				}
			}
			p.Mutex.Unlock()
		}
	}
}

type Order struct {
	OptType  messages.OperationType
	ZoneName string
	Values   map[string]string
}

func (p *Proposer) ReceiveOrder() {
	var (
		data = make([]byte, 1024)
	)
	for true {
		len, err := p.Conn.Read(data)
		if err != nil {
			fmt.Printf("[Run] Proposer read order failed err=%v\n", err)
			continue
		}
		p.OrderChan <- data[:len]
	}
}

func (p *Proposer) handleOrder(msg Order) {
	if proposal := messages.NewProposal(msg.ZoneName, msg.OptType, msg.Values); proposal != nil {
		proposalByte, err := json.Marshal(proposal)
		if err != nil {
			logger.Warningf("[handleOrder] json.Marshal error=%v", err)
			return
		}
		p.Mutex.Lock()
		p.Proposals[string(proposal.Id)] = *proposal
		p.Replys[string(proposal.Id)] = map[string]uint8{}
		ctx, cancelFunc := context.WithCancel(context.Background())
		go p.timer(ctx, proposal)
		p.Contexts[string(proposal.Id)] = cancelFunc
		p.Mutex.Unlock()
		service2.Net.BroadCast(proposalByte, service2.ProposalMsg)
	} else {
		logger.Warningf("[handleOrder] NewProposal failed")
	}
}

func (p *Proposer) timer(ctx context.Context, proposal *messages.ProposalMessage) {
	select {
	case <-time.After(conf.BCDnsConfig.ProposalTimeout):
		p.Mutex.Lock()
		defer p.Mutex.Unlock()
		replies, ok := p.Replys[string(proposal.Id)]
		if !ok {
			return
		}
		if service.CertificateAuthorityX509.Check(len(replies)) {
			fmt.Printf("[Proposer.timer] ProposalMsgT=%v execute successfully\n", string(proposal.Id))
			delete(p.Proposals, string(proposal.Id))
			delete(p.Replys, string(proposal.Id))
			delete(p.Contexts, string(proposal.Id))
		} else {
			confirmMsg := messages.NewProposalConfirm(proposal.Id)
			if confirmMsg == nil {
				logger.Warningf("[Proposer.timer] NewProposalConfirm failed")
				return
			}
			confirmMsgByte, err := json.Marshal(confirmMsg)
			if err != nil {
				logger.Warningf("[Proposer.timer] json.Marshal error=%v", err)
				return
			}
			service2.Net.BroadCast(confirmMsgByte, service2.ProposalConfirmMsg)
		}
	case <-ctx.Done():
		fmt.Println("[Proposer.timer] haha")
	}
}
