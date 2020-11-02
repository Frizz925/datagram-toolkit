package netem

import (
	"datagram-toolkit/util/errors"
	"io"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNetem(t *testing.T) {
	rand := rand.New(rand.NewSource(0))
	expectedLen := 64
	expected := make([]byte, expectedLen)
	buf := make([]byte, 256)
	cfg := Config{}

	var ns, nc *Netem
	{ // Setup
		require := require.New(t)
		_, err := io.ReadFull(rand, expected)
		require.Nil(err)

		s, err := net.ListenUDP("udp", nil)
		require.Nil(err)
		ns = New(s, cfg)

		c, err := net.DialUDP("udp", nil, s.LocalAddr().(*net.UDPAddr))
		require.Nil(err)
		nc = New(c, cfg)
	}

	clientWrite := func(require *require.Assertions) {
		w, err := nc.Write(expected)
		require.Nil(err)
		require.Equal(expectedLen, w)
	}

	serverRead := func(require *require.Assertions, fragmentSize int, timeout bool) int {
		if timeout {
			deadline := time.Now().Add(250 * time.Millisecond)
			require.Nil(ns.SetReadDeadline(deadline))
		}
		r := 0
		for r < expectedLen {
			n, err := ns.Read(buf[r:])
			if err != errors.ErrTimeout {
				require.Nil(err)
			} else {
				break
			}
			require.Equal(fragmentSize, n)
			r += n
		}
		return r
	}

	t.Run("normal", func(t *testing.T) {
		require := require.New(t)
		clientWrite(require)
		r, err := ns.Read(buf)
		require.Nil(err)
		require.Equal(expectedLen, r)
		require.Equal(expected, buf[:r])
	})

	t.Run("fragmentation:read", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			ReadFragmentSize: 16,
		}
		ns.Update(cfg)
		nc.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.ReadFragmentSize, false)
		require.Equal(expected, buf[:r])
	})

	t.Run("fragmentation:write", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			WriteFragmentSize: 16,
		}
		nc.Update(cfg)
		ns.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.WriteFragmentSize, false)
		require.Equal(expected, buf[:r])
	})

	t.Run("fragmentation+loss:read", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			ReadLossNth:      2,
			ReadFragmentSize: 16,
		}
		ns.Update(cfg)
		nc.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.ReadFragmentSize, true)
		require.Equal(r, expectedLen/2)
		require.Equal(expected[32:48], buf[16:32])
	})

	t.Run("fragmentation+loss:write", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			WriteLossNth:      2,
			WriteFragmentSize: 16,
		}
		nc.Update(cfg)
		ns.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.WriteFragmentSize, true)
		require.Equal(r, expectedLen/2)
		require.Equal(expected[32:48], buf[16:32])
	})

	t.Run("fragmentation+loss+duplication:read", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			ReadLossNth:      2,
			ReadDuplicateNth: 3,
			ReadFragmentSize: 16,
		}
		ns.Update(cfg)
		nc.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.ReadFragmentSize, true)
		require.Equal(r, expectedLen*3/4)
		require.Equal(buf[16:32], buf[32:48])
	})

	t.Run("fragmentation+loss+duplication:write", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			WriteLossNth:      2,
			WriteDuplicateNth: 3,
			WriteFragmentSize: 16,
		}
		nc.Update(cfg)
		ns.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.WriteFragmentSize, true)
		require.Equal(r, expectedLen*3/4)
		require.Equal(buf[16:32], buf[32:48])
	})

	{ // Teardown
		require := require.New(t)
		require.Nil(nc.Close())
		require.Nil(ns.Close())
	}
}
