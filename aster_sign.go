package cex

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
)

// Aster(v3)的鉴权: API Wallet(signer)私钥做EIP-712签名
// domain = {name:"AsterSignTransaction", version:"1", chainId:1666, verifyingContract:0x0}
// types  = EIP712Domain(name,version,chainId,verifyingContract) + Message(msg string)
// 被签名的msg是urlencode后的参数串(含nonce和signer), 签名结果放signature参数
const asChainID = 1666

var (
	asDomainTypeHash = asKeccak256([]byte("EIP712Domain(string name,string version," +
		"uint256 chainId,address verifyingContract)"))
	asMsgTypeHash = asKeccak256([]byte("Message(string msg)"))
	asNameHash    = asKeccak256([]byte("AsterSignTransaction"))
	asVerHash     = asKeccak256([]byte("1"))
)

func asKeccak256(b []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(b)
	return h.Sum(nil)
}

// asPrivKey 解析私钥, 支持带0x前缀
func asPrivKey(v string) (*secp256k1.PrivateKey, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(v, "0x"))
	if err != nil || len(b) != 32 {
		return nil, errors.New("aster invalid signer private key")
	}
	return secp256k1.PrivKeyFromBytes(b), nil
}

// asAddress 私钥对应的地址
func asAddress(priv *secp256k1.PrivateKey) string {
	return asPubKeyAddress(priv.PubKey())
}

// asPubKeyAddress 公钥对应的地址
func asPubKeyAddress(pub *secp256k1.PublicKey) string {
	raw := pub.SerializeUncompressed() // 65字节, 首字节是0x04
	sum := asKeccak256(raw[1:])
	return "0x" + hex.EncodeToString(sum[12:])
}

// asEip712Digest EIP-712摘要
func asEip712Digest(msg string) []byte {
	domain := make([]byte, 0, 160)
	domain = append(domain, asDomainTypeHash...)
	domain = append(domain, asNameHash...)
	domain = append(domain, asVerHash...)
	var chainId [32]byte
	binary.BigEndian.PutUint64(chainId[24:], asChainID)
	domain = append(domain, chainId[:]...)       // uint256
	domain = append(domain, make([]byte, 32)...) // address 0x0

	data := make([]byte, 0, 64)
	data = append(data, asMsgTypeHash...)
	data = append(data, asKeccak256([]byte(msg))...)

	digest := make([]byte, 0, 66)
	digest = append(digest, 0x19, 0x01)
	digest = append(digest, asKeccak256(domain)...)
	digest = append(digest, asKeccak256(data)...)
	return asKeccak256(digest)
}

// asSign 签名, 返回0x开头的65字节签名(r+s+v, v为27/28)
func asSign(priv *secp256k1.PrivateKey, msg string) string {
	if priv == nil { // 没配置API Wallet私钥
		return ""
	}
	compact := ecdsa.SignCompact(priv, asEip712Digest(msg), false)
	sig := make([]byte, 65)
	copy(sig[0:32], compact[1:33])
	copy(sig[32:64], compact[33:65])
	sig[64] = compact[0] // 27/28
	return "0x" + hex.EncodeToString(sig)
}
