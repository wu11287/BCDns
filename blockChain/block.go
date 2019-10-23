package blockChain

import (
	"BCDns_0.1/certificateAuthority/service"
	"BCDns_0.1/messages"
	"BCDns_0.1/utils"
	"bytes"
	"encoding/binary"
	"reflect"

	"github.com/izqui/functional"
	"github.com/izqui/helpers"
)

type BlockSlice []Block

func (bs BlockSlice) Exists(b Block) bool {

	//Traverse array in reverse order because if a block exists is more likely to be on top.
	l := len(bs)
	for i := l - 1; i >= 0; i-- {

		bb := bs[i]
		if reflect.DeepEqual(b.Signature, bb.Signature) {
			return true
		}
	}

	return false
}

func (bs BlockSlice) PreviousBlock() *Block {
	l := len(bs)
	if l == 0 {
		return nil
	} else {
		return &bs[l-1]
	}
}

type Block struct {
	*BlockHeader
	*messages.ProposalSlice
}

type BlockHeader struct {
	Origin     []byte
	PrevBlock  []byte
	MerkelRoot []byte
	Timestamp  uint32
	Nonce      uint32
}

func NewBlock(previousBlock []byte) Block {

	header := &BlockHeader{PrevBlock: previousBlock}
	return Block{header, new(messages.ProposalSlice)}
}

func (b *Block) AddTransaction(t *messages.ProposalMassage) {
	newSlice := b.ProposalSlice.AddTransaction(*t)
	b.ProposalSlice = &newSlice
}

func (b *Block) Sign() []byte {
	s := service.CertificateAuthorityX509.Sign(b.Hash())
	return s
}

func (b *Block) VerifyBlock(prefix []byte) bool {
	merkel := b.GenerateMerkelRoot()
	return reflect.DeepEqual(merkel, b.BlockHeader.MerkelRoot)
}

func (b *Block) Hash() []byte {

	headerHash, _ := b.BlockHeader.MarshalBinary()
	return utils.SHA256(headerHash)
}

func (b *Block) GenerateMerkelRoot() []byte {

	var merkell func(hashes [][]byte) []byte
	merkell = func(hashes [][]byte) []byte {

		l := len(hashes)
		if l == 0 {
			return nil
		}
		if l == 1 {
			return hashes[0]
		} else {

			if l%2 == 1 {
				return merkell([][]byte{merkell(hashes[:l-1]), hashes[l-1]})
			}

			bs := make([][]byte, l/2)
			for i, _ := range bs {
				j, k := i*2, (i*2)+1
				bs[i] = utils.SHA256(append(hashes[j], hashes[k]...))
			}
			return merkell(bs)
		}
	}

	ts := functional.Map(func(t Transaction) []byte { return t.Hash() }, []Transaction(*b.TransactionSlice)).([][]byte)
	return merkell(ts)

}
func (b *Block) MarshalBinary() ([]byte, error) {

	bhb, err := b.BlockHeader.MarshalBinary()
	if err != nil {
		return nil, err
	}
	sig := helpers.FitBytesInto(b.Signature, NETWORK_KEY_SIZE)
	tsb, err := b.TransactionSlice.MarshalBinary()
	if err != nil {
		return nil, err
	}

	return append(append(bhb, sig...), tsb...), nil
}

func (b *Block) UnmarshalBinary(d []byte) error {

	buf := bytes.NewBuffer(d)

	header := new(BlockHeader)
	err := header.UnmarshalBinary(buf.Next(BLOCK_HEADER_SIZE))
	if err != nil {
		return err
	}

	b.BlockHeader = header
	b.Signature = helpers.StripByte(buf.Next(NETWORK_KEY_SIZE), 0)

	ts := new(TransactionSlice)
	err = ts.UnmarshalBinary(buf.Next(helpers.MaxInt))
	if err != nil {
		return err
	}

	b.TransactionSlice = ts

	return nil
}

func (h *BlockHeader) MarshalBinary() ([]byte, error) {

	buf := new(bytes.Buffer)

	buf.Write(helpers.FitBytesInto(h.Origin, NETWORK_KEY_SIZE))
	binary.Write(buf, binary.LittleEndian, h.Timestamp)
	buf.Write(helpers.FitBytesInto(h.PrevBlock, 32))
	buf.Write(helpers.FitBytesInto(h.MerkelRoot, 32))
	binary.Write(buf, binary.LittleEndian, h.Nonce)

	return buf.Bytes(), nil
}

func (h *BlockHeader) UnmarshalBinary(d []byte) error {

	buf := bytes.NewBuffer(d)
	h.Origin = helpers.StripByte(buf.Next(NETWORK_KEY_SIZE), 0)
	binary.Read(bytes.NewBuffer(buf.Next(4)), binary.LittleEndian, &h.Timestamp)
	h.PrevBlock = buf.Next(32)
	h.MerkelRoot = buf.Next(32)
	binary.Read(bytes.NewBuffer(buf.Next(4)), binary.LittleEndian, &h.Nonce)

	return nil
}