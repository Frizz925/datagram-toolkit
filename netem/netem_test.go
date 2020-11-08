package netem

import (
	"datagram-toolkit/util/mocks"
	"errors"
	"io"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestNetem(t *testing.T) {
	rand := rand.New(rand.NewSource(0))
	expectedLen := 64
	expected := make([]byte, expectedLen)
	buf := make([]byte, 256)
	cfg := Config{}

	// Setup
	var ns, nc *Netem
	{
		require := require.New(t)
		_, err := io.ReadFull(rand, expected)
		require.Nil(err)

		s, c := mocks.Conn()
		ns = New(s, cfg)
		nc = New(c, cfg)
	}

	clientWrite := func(require *require.Assertions) {
		deadline := getOpDeadline()
		require.Nil(nc.SetDeadline(deadline))
		w, err := nc.Write(expected)
		require.Nil(err)
		require.Equal(expectedLen, w)
	}

	serverRead := func(require *require.Assertions, fragmentSize int, timeout bool) int {
		r := 0
		for r < expectedLen {
			if timeout {
				deadline := getOpDeadline()
				require.Nil(ns.SetDeadline(deadline))
			}
			n, err := ns.Read(buf[r:])
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					break
				}
				require.Fail("Unexpected error", "Expected deadline error, got %v", err)
			}
			require.GreaterOrEqual(fragmentSize, n)
			r += n
		}
		return r
	}

	// Teardown
	t.Cleanup(func() {
		require := require.New(t)
		require.Nil(nc.Close())
		require.Nil(ns.Close())
	})

	t.Run("normal", func(t *testing.T) {
		require := require.New(t)
		clientWrite(require)
		r, err := ns.Read(buf)
		require.Nil(err)
		require.Equal(expectedLen, r)
		require.Equal(expected, buf[:r])
	})

	t.Run("frag:read", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			ReadFragmentSize: 24,
		}
		ns.Update(cfg)
		nc.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.ReadFragmentSize, false)
		require.Equal(expected, buf[:r])
	})

	t.Run("frag:write", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			WriteFragmentSize: 24,
		}
		nc.Update(cfg)
		ns.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.WriteFragmentSize, false)
		require.Equal(expected, buf[:r])
	})

	t.Run("frag+loss:read", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			ReadFragmentSize: 16,
			ReadLossNth:      2,
		}
		ns.Update(cfg)
		nc.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.ReadFragmentSize, true)
		require.Equal(expectedLen/2, r)
		require.Equal(expected[32:48], buf[16:32])
	})

	t.Run("frag+loss:write", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			WriteFragmentSize: 16,
			WriteLossNth:      2,
		}
		nc.Update(cfg)
		ns.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.WriteFragmentSize, true)
		require.Equal(expectedLen/2, r)
		require.Equal(expected[32:48], buf[16:32])
	})

	t.Run("frag+dupe+loss:read", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			ReadFragmentSize: 16,
			ReadDuplicateNth: 2,
			ReadLossNth:      4,
		}
		ns.Update(cfg)
		nc.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.ReadFragmentSize, true)
		require.Equal(expectedLen, r)
		require.Equal(expected[:32], buf[:32])
		require.Equal(buf[16:32], buf[32:48])
		require.Equal(expected[48:], buf[48:64])
	})

	t.Run("frag+loss+dupe:write", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			WriteFragmentSize: 16,
			WriteDuplicateNth: 2,
			WriteLossNth:      4,
		}
		nc.Update(cfg)
		ns.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.WriteFragmentSize, true)
		require.Equal(expectedLen, r)
		require.Equal(expected[:32], buf[:32])
		require.Equal(buf[16:32], buf[32:48])
		require.Equal(expected[48:], buf[48:64])
	})

	t.Run("frag+dupe+reorder+loss:read", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			ReadFragmentSize: 16,
			ReadDuplicateNth: 2,
			ReadReorderNth:   3,
			ReadLossNth:      4,
		}
		ns.Update(cfg)
		nc.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.ReadFragmentSize, true)
		require.Equal(expectedLen, r)
		require.Equal(expected[:48], buf[:48])
		require.Equal(expected[16:32], buf[48:64])
	})

	t.Run("frag+dupe+reorder+loss:write", func(t *testing.T) {
		require := require.New(t)
		cfg := Config{
			WriteFragmentSize: 16,
			WriteDuplicateNth: 2,
			WriteReorderNth:   3,
			WriteLossNth:      4,
		}
		nc.Update(cfg)
		ns.Reset()
		clientWrite(require)
		r := serverRead(require, cfg.WriteFragmentSize, true)
		require.Equal(expectedLen, r)
		require.Equal(expected[:48], buf[:48])
		require.Equal(expected[16:32], buf[48:64])
	})
}

func getOpDeadline() time.Time {
	return time.Now().Add(125 * time.Millisecond)
}

func init() {
	log.Out = os.Stdout
	log.Level = logrus.DebugLevel
}
