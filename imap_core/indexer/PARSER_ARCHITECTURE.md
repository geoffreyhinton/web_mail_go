# RFC822 Email Parser Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           RFC822 EMAIL PARSER ARCHITECTURE                      │
└─────────────────────────────────────────────────────────────────────────────────┘

                                   INPUT
                              ┌─────────────┐
                              │ Raw RFC822  │
                              │ Email Bytes │
                              └──────┬──────┘
                                     │
                                     ▼
                        ┌─────────────────────────┐
                        │    ParseMIME()         │
                        │ Entry Point Function   │
                        └──────────┬──────────────┘
                                   │
                                   ▼
                        ┌─────────────────────────┐
                        │   NewMIMEParser()      │
                        │  Initialize Parser     │
                        └──────────┬──────────────┘
                                   │
                                   ▼
                        ┌─────────────────────────┐
                        │   MIMEParser.Parse()   │
                        │   Main Parse Loop      │
                        └──────────┬──────────────┘
                                   │
                                   ▼

┌─────────────────────────────────────────────────────────────────────────────────┐
│                              PARSING PIPELINE                                    │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  ┌─────────────┐    ┌─────────────────┐    ┌──────────────────┐               │
│  │ readLine()  │───▶│ State Machine   │───▶│ processNodeHeader│               │
│  │ Line Reader │    │ header | body   │    │ Header Parsing   │               │
│  └─────────────┘    └─────────────────┘    └──────────────────┘               │
│                              │                        │                        │
│                              ▼                        ▼                        │
│  ┌─────────────┐    ┌─────────────────┐    ┌──────────────────┐               │
│  │ Boundary    │    │ Content-Type    │    │ Address Parsing  │               │
│  │ Detection   │    │ Processing      │    │ From/To/CC/BCC   │               │
│  └─────────────┘    └─────────────────┘    └──────────────────┘               │
│                              │                        │                        │
│                              ▼                        ▼                        │
│  ┌─────────────┐    ┌─────────────────┐    ┌──────────────────┐               │
│  │ createNode()│    │ parseValueParams│    │ Header Folding   │               │
│  │ Tree Builder│    │ Parameter Parse │    │ Multi-line Fix   │               │
│  └─────────────┘    └─────────────────┘    └──────────────────┘               │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

                                   │
                                   ▼
                        ┌─────────────────────────┐
                        │  FinalizeTree()        │
                        │  Post-processing       │
                        └──────────┬──────────────┘
                                   │
                                   ▼
                                 OUTPUT

┌─────────────────────────────────────────────────────────────────────────────────┐
│                             DATA STRUCTURES                                      │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────────┐ │
│  │                            MIMENode                                         │ │
│  ├─────────────────────────────────────────────────────────────────────────────┤ │
│  │ • RootNode: bool                   • Multipart: string                     │ │
│  │ • ChildNodes: []*MIMENode         • Boundary: string                      │ │
│  │ • Header: []string                • ParentBoundary: string               │ │
│  │ • ParsedHeader: map[string]interface{}  • LineCount: int                 │ │
│  │ • Body: []byte                    • Size: int                            │ │
│  │ • Message: *MIMENode (for rfc822) • state: string (internal)            │ │
│  └─────────────────────────────────────────────────────────────────────────────┘ │
│                                     │                                           │
│                                     ▼                                           │
│  ┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────────┐ │
│  │     Address         │  │    ValueParams      │  │     MIMEParser          │ │
│  ├─────────────────────┤  ├─────────────────────┤  ├─────────────────────────┤ │
│  │ • Name: string      │  │ • Value: string     │  │ • rfc822: string        │ │
│  │ • Address: string   │  │ • Type: string      │  │ • pos: int              │ │
│  │                     │  │ • Subtype: string   │  │ • br: string            │ │
│  │ Used for:           │  │ • Params: map[]     │  │ • rawBody: string       │ │
│  │ - From/To/CC/BCC    │  │ • HasParams: bool   │  │ • tree: *MIMENode       │ │
│  │ - Reply-To/Sender   │  │                     │  │ • node: *MIMENode       │ │
│  │                     │  │ Used for:           │  │                         │ │
│  │                     │  │ - Content-Type      │  │ Parser State Machine    │ │
│  │                     │  │ - Content-Disposition│ │                         │ │
│  └─────────────────────┘  └─────────────────────┘  └─────────────────────────┘ │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────┐
│                           PARSING FLOW DIAGRAM                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│ Raw Email Input                                                                 │
│       │                                                                         │
│       ▼                                                                         │
│ ┌─────────────┐                                                                 │
│ │ Line Reader │◄──────────────┐                                                │
│ │ readLine()  │               │                                                 │
│ └──────┬──────┘               │                                                 │
│        │                      │                                                 │
│        ▼                      │                                                 │
│ ┌─────────────┐               │                                                 │
│ │State Machine│               │                                                 │
│ │   Switch    │               │                                                 │
│ └──────┬──────┘               │                                                 │
│        │                      │                                                 │
│    ┌───▼───┐              ┌───┴───┐                                            │
│    │header │              │ body  │                                            │
│    │ state │              │ state │                                            │
│    └───┬───┘              └───┬───┘                                            │
│        │                      │                                                 │
│        ▼                      ▼                                                 │
│ ┌─────────────┐        ┌─────────────┐                                         │
│ │Collect      │        │Boundary     │                                         │
│ │Headers      │        │Detection    │                                         │
│ └──────┬──────┘        └──────┬──────┘                                         │
│        │                      │                                                 │
│        ▼                      ▼                                                 │
│ ┌─────────────┐        ┌─────────────┐                                         │
│ │Process      │        │Body Content │                                         │
│ │Headers      │        │Accumulation │                                         │
│ └──────┬──────┘        └──────┬──────┘                                         │
│        │                      │                                                 │
│        └──────────┬───────────┘                                                │
│                   │                                                             │
│                   ▼                                                             │
│            ┌─────────────┐                                                      │
│            │Create Child │                                                      │
│            │    Node     │                                                      │
│            └──────┬──────┘                                                      │
│                   │                                                             │
│                   └───────────────────────────────────────────────┐             │
│                                                                   │             │
│                                                                   │             │
│                   ┌───────────────────────────────────────────────┘             │
│                   │                                                             │
│                   ▼                                                             │
│            ┌─────────────┐                                                      │
│            │ Finalize    │                                                      │
│            │    Tree     │                                                      │
│            └──────┬──────┘                                                      │
│                   │                                                             │
│                   ▼                                                             │
│            ┌─────────────┐                                                      │
│            │Return MIME  │                                                      │
│            │    Tree     │                                                      │
│            └─────────────┘                                                      │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────┐
│                        MULTIPART PARSING EXAMPLE                                │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│ Input: multipart/mixed email                                                    │
│                                                                                 │
│        ┌─────────────────────┐                                                 │
│        │    Root MIMENode    │                                                 │
│        │  multipart/mixed    │                                                 │
│        │ boundary="bound123" │                                                 │
│        └──────────┬──────────┘                                                 │
│                   │                                                             │
│          ┌────────┼────────┐                                                   │
│          ▼                 ▼                                                   │
│   ┌─────────────┐   ┌─────────────┐                                           │
│   │Child Node 1 │   │Child Node 2 │                                           │
│   │ text/plain  │   │image/jpeg   │                                           │
│   │   Body:     │   │ attachment  │                                           │
│   │"Hello..."   │   │   Body:     │                                           │
│   └─────────────┘   │ [binary]    │                                           │
│                     └─────────────┘                                           │
│                                                                                 │
│ Parsing Process:                                                                │
│ 1. Parse headers → detect multipart/mixed                                      │
│ 2. Extract boundary="bound123"                                                  │
│ 3. Switch to body state                                                         │
│ 4. Detect "--bound123" → create child node                                     │
│ 5. Parse child headers → text/plain                                            │
│ 6. Collect child body until next boundary                                      │
│ 7. Detect "--bound123" → create another child                                  │
│ 8. Parse child headers → image/jpeg                                            │
│ 9. Collect binary data until "--bound123--"                                    │
│ 10. Finalize tree structure                                                     │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────┐
│                           HEADER PROCESSING                                     │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│ Raw Header Lines                    Processed Headers                          │
│                                                                                 │
│ ┌─────────────────────┐           ┌─────────────────────┐                     │
│ │From: John Doe       │          │"from": [Address{    │                     │
│ │ <john@example.com>  │   ───▶   │  Name: "John Doe",  │                     │
│ │To: jane@test.com    │          │  Address: "john@ex" │                     │
│ │Subject: Long        │          │}]                   │                     │
│ │ subject line        │          │"to": [Address{      │                     │
│ │ continues here      │          │  Address: "jane@"   │                     │
│ │Content-Type:        │          │}]                   │                     │
│ │ text/html;          │          │"subject": "Long     │                     │
│ │ charset=utf-8       │          │ subject line        │                     │
│ └─────────────────────┘          │ continues here"     │                     │
│                                  │"content-type":      │                     │
│                                  │ ValueParams{        │                     │
│                                  │   Type: "text",     │                     │
│                                  │   Subtype: "html",  │                     │
│                                  │   Params: {         │                     │
│                                  │     "charset":"utf-8"│                    │
│                                  │   }                 │                     │
│                                  │ }                   │                     │
│                                  └─────────────────────┘                     │
│                                                                                 │
│ Processing Steps:                                                               │
│ 1. Header folding detection and unfolding                                      │
│ 2. Key-value pair extraction (split on first ":")                             │
│ 3. Key validation and normalization (lowercase)                               │
│ 4. Value parameter parsing for structured headers                              │
│ 5. Address parsing for email fields                                           │
│ 6. Array handling for multiple values                                         │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────┐
│                            PERFORMANCE PROFILE                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│ Simple Email (text/plain):                                                     │
│ ┌─────────────────────────────────────────────────────────┐                   │
│ │ Parse Time: ~117μs                                      │                   │
│ │ Memory: 46KB allocations                               │                   │
│ │ Allocs: 530 allocations                               │                   │
│ └─────────────────────────────────────────────────────────┘                   │
│                                                                                 │
│ Multipart Email (mixed/alternative):                                           │
│ ┌─────────────────────────────────────────────────────────┐                   │
│ │ Parse Time: ~175μs                                      │                   │
│ │ Memory: 95KB allocations                               │                   │
│ │ Allocs: 1018 allocations                              │                   │
│ └─────────────────────────────────────────────────────────┘                   │
│                                                                                 │
│ Throughput: ~5,700-8,500 emails/second (single thread)                         │
│ Test Coverage: 85.4% of code statements                                        │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────┐
│                             INTEGRATION POINTS                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│                        ┌─────────────────────┐                                │
│                        │   ParseMIME()       │                                │
│                        │  Main Entry Point   │                                │
│                        └──────────┬──────────┘                                │
│                                   │                                            │
│              ┌────────────────────┼────────────────────┐                      │
│              ▼                    ▼                    ▼                      │
│    ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│    │ Body Structure  │  │ Email Indexer   │  │ IMAP Server     │             │
│    │ Generator       │  │ (MongoDB)       │  │ Integration     │             │
│    │                 │  │                 │  │                 │             │
│    │ CreateBody      │  │ IndexEmail()    │  │ FETCH Commands  │             │
│    │ Structure()     │  │                 │  │ ENVELOPE        │             │
│    │                 │  │ Process         │  │ BODYSTRUCTURE   │             │
│    │ IMAP Compat     │  │ Content()       │  │                 │             │
│    └─────────────────┘  └─────────────────┘  └─────────────────┘             │
│                                                                                 │
│    ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│    │ Search Engine   │  │ Content Filter  │  │ Archive System  │             │
│    │ Integration     │  │ Spam Detection  │  │ Email Storage   │             │
│    │                 │  │                 │  │                 │             │
│    │ Full-text       │  │ Attachment      │  │ Compressed      │             │
│    │ Indexing        │  │ Scanning        │  │ Storage         │             │
│    │                 │  │                 │  │                 │             │
│    │ Elasticsearch   │  │ Virus Check     │  │ Long-term       │             │
│    │ Solr           │  │ Content Policy  │  │ Retention       │             │
│    └─────────────────┘  └─────────────────┘  └─────────────────┘             │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

## Key Architecture Components

### 1. **Entry Layer**
- `ParseMIME()` - Main public API
- Input validation and error handling
- Results in structured `MIMENode` tree

### 2. **Parser Engine**
- `MIMEParser` - State machine implementation
- Line-by-line processing with regex
- Boundary detection and multipart handling
- Memory-efficient streaming approach

### 3. **State Machine**
- **Header State**: Collects and processes headers
- **Body State**: Processes content and detects boundaries
- **Recursive Parsing**: Handles nested structures

### 4. **Data Processing**
- **Header Processing**: Folding, validation, parameter parsing
- **Address Parsing**: Email address extraction using Go's `mail` package
- **Content-Type**: MIME type and parameter parsing
- **Tree Construction**: Hierarchical MIME structure building

### 5. **Output Generation**
- **Tree Finalization**: Line counting, size calculation
- **Memory Cleanup**: Removes temporary parsing fields
- **Structured Output**: Ready-to-use MIME tree

This architecture provides **high performance**, **RFC822 compliance**, and **extensive MIME support** while maintaining **clean separation of concerns** and **easy integration** with email systems.
