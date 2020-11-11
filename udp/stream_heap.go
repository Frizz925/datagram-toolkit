package udp

type streamRecvPendingHeap []streamRecvPending

func (h streamRecvPendingHeap) Len() int           { return len(h) }
func (h streamRecvPendingHeap) Less(i, j int) bool { return h[i].off < h[j].off }
func (h streamRecvPendingHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *streamRecvPendingHeap) Push(x interface{}) {
	*h = append(*h, x.(streamRecvPending))
}

func (h *streamRecvPendingHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type streamRetransmitPendingHeap []streamRetransmitPending

func (h streamRetransmitPendingHeap) Len() int           { return len(h) }
func (h streamRetransmitPendingHeap) Less(i, j int) bool { return h[i].seq < h[j].seq }
func (h streamRetransmitPendingHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *streamRetransmitPendingHeap) Push(x interface{}) {
	*h = append(*h, x.(streamRetransmitPending))
}

func (h *streamRetransmitPendingHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
