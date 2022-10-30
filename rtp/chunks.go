package rtp

import "bytes"

func chunksLocate(chunks [][]byte, index uint) (uint, uint) {
	for i, chunk := range chunks {
		if index < uint(len(chunk)) {
			return uint(i), index
		}
		index -= uint(len(chunk))
	}
	return uint(len(chunks)), 0
}

func chunksCutBytesFrom(chunks [][]byte, sx, sy uint) []byte {
	if sx == uint(len(chunks)) {
		return nil
	} else if sx == uint(len(chunks))-1 {
		return chunks[sx][sy:]
	} else {
		old := chunks[sx]
		chunks[sx] = chunks[sx][sy:]
		res := bytes.Join(chunks[sx:], nil)
		chunks[sx] = old
		return res
	}
}

func ChunksCutBytesFrom(chunks [][]byte, start uint) []byte {
	sx, sy := chunksLocate(chunks, start)
	return chunksCutBytesFrom(chunks, sx, sy)
}

func chunksCutBytesTo(chunks [][]byte, ex, ey uint) []byte {
	if ex == uint(len(chunks)) {
		return bytes.Join(chunks, nil)
	} else if ex == 0 {
		return chunks[0][:ey]
	} else {
		o := chunks[ex]
		chunks[ex] = chunks[ex][:ey]
		res := bytes.Join(chunks[:ex+1], nil)
		chunks[ex] = o
		return res
	}
}

func ChunksCutBytesTo(chunks [][]byte, end uint) []byte {
	ex, ey := chunksLocate(chunks, end)
	return chunksCutBytesTo(chunks, ex, ey)
}

func chunksCutChunksFrom(chunks [][]byte, sx, sy uint) [][]byte {
	if sx == uint(len(chunks)) {
		return nil
	} else if sx == uint(len(chunks))-1 {
		return [][]byte{chunks[sx][sy:]}
	} else {
		res := append([][]byte(nil), chunks[sx:]...)
		res[0] = res[0][sy:]
		return res
	}
}

func ChunksCutChunksFrom(chunks [][]byte, start uint) [][]byte {
	sx, sy := chunksLocate(chunks, start)
	return chunksCutChunksFrom(chunks, sx, sy)
}

func chunksCutChunksTo(chunks [][]byte, ex, ey uint) [][]byte {
	if ex == uint(len(chunks)) {
		return chunks
	} else if ex == 0 {
		res := chunks[ex][:ey]
		if len(res) == 0 {
			return nil
		} else {
			return [][]byte{res}
		}
	} else {
		res := append([][]byte(nil), chunks[:ex+1]...)
		res[ex] = res[ex][:ey]
		if len(res[ex]) == 0 {
			res = res[:ex]
		}
		return res
	}
}

func ChunksCutChunksTo(chunks [][]byte, end uint) [][]byte {
	ex, ey := chunksLocate(chunks, end)
	return chunksCutChunksTo(chunks, ex, ey)
}

func SplitIntoBytesLinear(chunks [][]byte, index uint) ([]byte, [][]byte) {
	x, y := chunksLocate(chunks, index)
	return chunksCutBytesTo(chunks, x, y), chunksCutChunksFrom(chunks, x, y)
}

func ChunksLastByte(chunks [][]byte) byte {
	last := chunks[len(chunks)-1]
	return last[len(last)-1]
}
