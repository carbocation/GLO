package glo

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Represents a contig:start-end structure.
type ChainInterval struct {
	Contig   string
	Start    int64
	End      int64
	Inverted bool
}

func (ci ChainInterval) size() int64 {
	return ci.End - ci.Start
}

func NewChainInterval(contig string, start int64, end int64) *ChainInterval {
	ci := new(ChainInterval)
	ci.Contig = contig
	ci.Start = start
	ci.End = end
	return ci
}

func (ci ChainInterval) String() string {
	return string(fmt.Sprintf("%s:%d-%d", ci.Contig, ci.Start, ci.End))
}

// Represents the source -> target mapping between
// two ChainIntervals, e.g.
// chrA:10000-20000 -> chrB:20123-30123
type ChainLink struct {
	reference *ChainInterval
	query     *ChainInterval
	chain     *Chain
	size      int64
	dt        int64
	dq        int64
}

// String output fuction for ChainLink type
func (link *ChainLink) String() string {
	return fmt.Sprintf("%s <-> %s", link.reference, link.query)
}

// OUtputs the original chain file format line
func (link *ChainLink) Line() string {
	if link.dq == 0 && link.dt == 0 {
		return fmt.Sprintf("%d", link.size)
	} else {
		return fmt.Sprintf("%d %d %d", link.size, link.dt, link.dq)
	}
}

// GetOverlap returns a ChainInterval object representing the
// overlap with the input region.
func (link *ChainLink) GetOverlap(region *ChainInterval) *ChainInterval {
	if region.Contig != link.reference.Contig {
		// No overlap due to contig mismatch
		return nil
	}

	var start_offset, end_offset int64 = 0, 0

	// Adjust the start offset of the overlap, in case the region
	// starts at a later position that the overlapping reference block.
	if region.Start > link.reference.Start {
		start_offset = region.Start - link.reference.Start
	}

	// Likewise, adjust the end offset in case there is less than the
	// full block's worth of overlap
	if region.End < link.reference.End {
		end_offset = link.reference.End - region.End
	}

	overlap := new(ChainInterval)
	overlap.Contig = link.query.Contig
	overlap.Start = link.query.Start + start_offset
	overlap.End = link.query.End - end_offset
	overlap.Inverted = link.query.Inverted
	// fmt.Printf("%s\n", link.chain.Header())
	// fmt.Printf("%s\t%s\n", link, overlap)
	return overlap
}

// The Chain type represents a UCSC chain object, including all
// the fields from the header line and each block of mappings
// for that chain.
type Chain struct {
	score   int64
	tName   string
	tSize   int64
	tStrand string
	tStart  int64
	tEnd    int64
	qName   string
	qSize   int64
	qStrand string
	qStart  int64
	qEnd    int64
	id      string
	links   []*ChainLink
}

func (c *Chain) Header() string {
	return fmt.Sprintf("chain %d %s %d %s %d %d %s %d %s %d %d %s",
		c.score, c.tName, c.tSize, c.tStrand,
		c.tStart, c.tEnd, c.qName, c.qSize,
		c.qStrand, c.qStart, c.qEnd, c.id)
}

// String output function for Chain type.
func (c *Chain) String() string {
	var output []string
	output = append(output,
		fmt.Sprintf("%s:%d%s%d to %s:%d%s%d", c.tName, c.tStart, c.tStrand,
			c.tEnd, c.qName, c.qStart, c.qStrand, c.qEnd))
	for _, link := range c.links {
		output = append(output, fmt.Sprintf("> %s", link))
	}
	return strings.Join(output, "\n")
}

// Populates the target Chain struct from the data in the input string.
func (c *Chain) FromString(s string) {
	cols := strings.Split(strings.TrimSpace(s), " ")
	c.score = str2int64(cols[1])
	c.tName = strings.ToLower(cols[2])
	c.tSize = str2int64(cols[3])
	c.tStrand = cols[4]
	c.tStart = str2int64(cols[5])
	c.tEnd = str2int64(cols[6])
	c.qName = strings.ToLower(cols[7])
	c.qSize = str2int64(cols[8])
	c.qStrand = cols[9]
	c.qStart = str2int64(cols[10])
	c.qEnd = str2int64(cols[11])
	if len(cols) == 13 {
		c.id = cols[12]
	}
}

// Chain.load_links uses the input bufio.Reader to read mapping
// block from the file, until the end of the chain is found. The
// mapping links are added to the Chain as ChainLink structs.
func (c *Chain) load_links(reader *bufio.Reader) {
	var cols []string
	var link *ChainLink

	inverted := (c.tStrand != c.qStrand)

	line, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("Chain.load_links() error: %s\n", err)
		os.Exit(1)
	}
	cols = strings.Split(strings.TrimSpace(string(line)), "\t")

	tFrom := c.tStart
	qFrom := c.qStart
	if inverted {
		qFrom = c.qSize - c.qStart
	}

	for len(cols) == 3 {
		size := str2int64(cols[0])
		dt := str2int64(cols[1])
		dq := str2int64(cols[2])

		link = new(ChainLink)
		link.chain = c
		link.size = size
		link.dt = dt
		link.dq = dq

		link.reference = NewChainInterval(c.tName, tFrom, tFrom+size)
		link.reference.Inverted = inverted

		tFrom += size + dt
		if !inverted {
			// Regular orientation; handle as usual
			link.query = NewChainInterval(c.qName, qFrom, qFrom+size)
			link.query.Inverted = inverted
			qFrom += size + dq
		} else {
			// Inverse orientation; handle on complementary strand
			link.query = NewChainInterval(c.qName, qFrom-size-1, qFrom)
			link.query.Inverted = inverted
			qFrom -= (size + dq)
		}
		c.links = append(c.links, link)

		_, p_err := reader.Peek(1)
		if p_err != nil {
			// EOF
			break
		}
		line, err = reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Chain.load_blocks() error: %s\n", err)
			os.Exit(1)
		}
		cols = strings.Split(strings.TrimSpace(string(line)), "\t")
	}

	if len(cols) != 1 {
		fmt.Printf("Error: Expected line with a single value, got \"%s\"\n", cols)
		os.Exit(1)
	}

	size := str2int64(cols[0])
	link = new(ChainLink)
	link.chain = c
	link.size = size

	link.reference = NewChainInterval(c.tName, tFrom, tFrom+size)
	link.reference.Inverted = inverted

	if !inverted {
		link.query = NewChainInterval(c.qName, qFrom, qFrom+size)
		link.query.Inverted = inverted
	} else {
		link.query = NewChainInterval(c.qName, qFrom-size-1, qFrom)
		link.query.Inverted = inverted
	}

	c.links = append(c.links, link)

}
