package service

import (
	"BCDns_0.1/bcDns/conf"
	"BCDns_0.1/certificateAuthority/service"
	"BCDns_0.1/dao"
	"BCDns_0.1/messages"
	"BCDns_0.1/utils"
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"time"
)

type OperationType uint8

const (
	Add OperationType = iota
	Del
	Mod
)

type Base struct {
	From string
	TimeStamp int64
}

type ProposalMessage struct {
	Base
	Type OperationType
	ZoneName string
	Owner string
	Values    map[string]string
	Nonce uint32
	Id        []byte
	Signature []byte
}

func NewProposal(zoneName string, t OperationType, values map[string]string) *ProposalMessage {
	var (
		err error
		base = Base{
			From: conf.BCDnsConfig.HostName,
			TimeStamp:time.Now().Unix(),
		}
		msg ProposalMessage
	)
	switch t {
	case Add:
		msg = ProposalMessage{
			Base: base,
			Type: Add,
			ZoneName:zoneName,
			Owner:base.From,
			Values:values,
		}
		err = msg.GetPow()
		if err != nil {
			fmt.Printf("[NewProposal] GetPowHash Failed err=%v\n", err)
			return nil
		}
	case Del:
		msg = ProposalMessage{
			Base: base,
			Type: Del,
			ZoneName:zoneName,
			Owner:messages.Dereliction,
			Values:values,
		}
	case Mod:
		blockProposal := new(ProposalMessage)
		if data, err := dao.Dao.GetZoneName(zoneName); err == leveldb.ErrNotFound {
			fmt.Printf("[ValidateMod] ZoneName is not exist\n")
			return nil
		} else {
			blockProposal, err = UnMarshalProposalMessage(data)
			if err != nil {
				fmt.Printf("[NewProposal] UnMarshalProposalMassage error=%v\n", err)
				return nil
			}
			if blockProposal.From == messages.Dereliction {
				fmt.Printf("[ValidateMod] ZoneName is not exist\n")
				return nil
			}
		}
		msg = ProposalMessage{
			Base:base,
			Type:Mod,
			ZoneName:zoneName,
			Owner: base.From,
			Values:utils.CoverMap(blockProposal.Values, values),
		}
	default:
		fmt.Println("Unknown proposal type")
		return nil
	}
	msg.Id, err = msg.Hash()
	if err != nil {
		fmt.Printf("[NewProposal] Hash Failed err=%v\n", err)
		return nil
	}
	err = msg.Sign()
	if err != nil {
		fmt.Printf("[NewProposal] msg.Sign error=%v\n", err)
		return nil
	}
	return &msg
}

func (msg *ProposalMessage) GetPow() error {
	for {
		hash, err := msg.Hash()
		if err != nil {
			return err
		}
		if utils.CheckProofOfWork(utils.ProposalPOW, hash) {
			break
		} else {
			msg.Nonce++
		}
	}
	return nil
}

func (msg *ProposalMessage) Hash() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(msg.Base); err != nil {
		return nil, err
	}
	if err := enc.Encode(msg.Type); err != nil {
		return nil, err
	}
	if err := enc.Encode(msg.ZoneName); err != nil {
		return nil, err
	}
	if err := enc.Encode(msg.Owner); err != nil {
		return nil, err
	}
	if err := enc.Encode(msg.Values); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (msg *ProposalMessage) Sign() error {
	hash, err := msg.Hash()
	if err != nil {
		return err
	}
	if sig := service.CertificateAuthorityX509.Sign(hash); err != nil {
		msg.Signature = sig
		return nil
	}
	return errors.New("[ProposalMessage.Sign] generate signature failed")
}

func (msg *ProposalMessage) VerifySignature() bool {
	hash, err := msg.Hash()
	if err != nil {
		return false
	}
	return service.CertificateAuthorityX509.VerifySignature(msg.Signature, hash, msg.From)
}

func (msg *ProposalMessage) MarshalProposalMessage() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(msg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func UnMarshalProposalMessage(data []byte) (*ProposalMessage, error) {
	msg := new(ProposalMessage)
	dec := gob.NewDecoder(bytes.NewBuffer(data))
	if err := dec.Decode(msg); err != nil {
		return nil, err
	}
	return msg, nil
}

type ProposalConfirm struct {
	Base
	ProposalHash []byte
	Signature []byte
}

func NewProposalConfirm(proposalHash []byte) *ProposalConfirm {
	msg := ProposalConfirm{
		Base:Base{
			From:conf.BCDnsConfig.HostName,
			TimeStamp:time.Now().Unix(),
		},
		ProposalHash:proposalHash,
	}
	if err := msg.Sign(); err != nil {
		return nil
	}
	return &msg
}

func (msg *ProposalConfirm) Hash() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(msg.Base); err != nil {
		return nil, err
	}
	if err := enc.Encode(msg.ProposalHash); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (msg *ProposalConfirm) Sign() error {
	hash, err := msg.Hash()
	if err != nil {
		return err
	}
	if sig := service.CertificateAuthorityX509.Sign(hash); err != nil {
		msg.Signature = sig
		return nil
	}
	return errors.New("[ProposalConfirm.Sign] generate signature failed")
}

func (msg *ProposalConfirm) VerifySignature() bool {
	hash, err := msg.Hash()
	if err != nil {
		return false
	}
	return service.CertificateAuthorityX509.VerifySignature(msg.Signature, hash, msg.From)
}