package tools

// reactionTypes are the backend REACTION_TYPES — the kind of reaction/event
// placed under an actor. `comment` is the everyday "post a note" type; `ai` is
// the AI agent's reply, normally produced by the platform's agent (see the
// `extra.mcp` flag and the AI-agent flow in docs/entities/reactions.md).
var reactionTypes = []string{"view", "comment", "ai", "rating", "sign", "ds", "done", "reject", "freeze"}

// reactionExtraDesc documents the optional `extra` presentation object.
const reactionExtraDesc = "Optional presentation object: " +
	"{commentStyleType:\"primary\"|\"secondary\"|\"text\", linkedActorId:string, " +
	"layerPosition:{x:number,y:number}, mcp:boolean}. " +
	"mcp:true hands the reaction to the AI agent — it processes the reaction under " +
	"the requesting user's access (via this MCP server) and posts its answer back " +
	"as a child `ai` reaction. See docs/entities/reactions.md."

// reactionReasoningDesc documents the optional `reasoning` object — the AI
// agent's progress/answer trace, carried on `ai` reactions.
const reactionReasoningDesc = "Optional AI-agent trace: " +
	"{inProgress:boolean, thoughts:[{id:string,text:string,createdAt:number}]}. " +
	"On `ai` reactions the agent streams progress here (inProgress=true while " +
	"running, thoughts = step log) while the answer text fills `description`."

// reactionOps — reactions are events/comments that live under a parent actor
// (and are themselves actors). In every path, `{actorId}` is the PARENT (root)
// actor whose reaction tree is addressed; a reaction's OWN id travels in the body
// (exposed here as the `reactionId` argument) for update/delete/pin.
var reactionOps = []Operation{
	{
		Name: "createReaction", Method: "POST", Path: "/reactions/{type}/{actorId}",
		Summary: "Add a reaction (comment/event) under an actor. Use type=comment for a plain text note. " +
			"The reaction becomes a child of the actor; reply to another reaction by passing its id as parentId.",
		Params: []Param{
			{Name: "type", In: InPath, Type: "string", Required: true, Enum: reactionTypes, Desc: "Reaction kind (comment for a note; ai is the agent's reply, usually platform-produced)."},
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Parent (root) actor UUID the reaction is placed under."},
			{Name: "description", In: InBody, Type: "string", Desc: "The reaction text (e.g. the comment body)."},
			{Name: "data", In: InBody, Type: "object", Desc: "Optional structured payload carried on the reaction."},
			{Name: "parentId", In: InBody, Type: "string", Desc: "Optional id of the reaction this one replies to (threading)."},
			{Name: "hidden", In: InBody, Type: "boolean", Desc: "Create the reaction hidden."},
			{Name: "extra", In: InBody, Type: "object", Desc: reactionExtraDesc},
			{Name: "reasoning", In: InBody, Type: "object", Desc: reactionReasoningDesc},
			{Name: "attachments", In: InBody, Type: "array", Desc: "Optional attachments: array (1-20) of {attachId:int} (ids from uploadBase64 / getAttachments)."},
			{Name: "notify", In: InQuery, Type: "boolean", KeepFalse: true, Desc: "Send notifications about the new reaction (default true). Set false to post quietly."},
			{Name: "sipTranscription", In: InQuery, Type: "boolean", Desc: "Mark the reaction as a SIP transcription entry."},
		},
	},
	{
		Name: "updateReaction", Method: "PUT", Path: "/reactions/{actorId}",
		Summary: "Edit an existing reaction. Identify the reaction by reactionId; actorId is its parent (root) actor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Parent (root) actor UUID the reaction belongs to."},
			{Name: "reactionId", In: InBody, Wire: "actorId", Type: "string", Required: true, Desc: "UUID of the reaction to update."},
			{Name: "description", In: InBody, Type: "string", Desc: "New reaction text."},
			{Name: "data", In: InBody, Type: "object", Desc: "Replacement structured payload."},
			{Name: "parentId", In: InBody, Type: "string", Desc: "New parent reaction id (re-thread)."},
			{Name: "hidden", In: InBody, Type: "boolean", Desc: "Hide/unhide the reaction."},
			{Name: "extra", In: InBody, Type: "object", Desc: reactionExtraDesc},
			{Name: "reasoning", In: InBody, Type: "object", Desc: reactionReasoningDesc},
		},
	},
	{
		Name: "deleteReaction", Method: "DELETE", Path: "/reactions/{actorId}",
		Summary: "Delete a reaction. Identify it by reactionId; actorId is its parent (root) actor. Irreversible — confirm first.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Parent (root) actor UUID the reaction belongs to."},
			{Name: "reactionId", In: InBody, Wire: "actorId", Type: "string", Required: true, Desc: "UUID of the reaction to delete."},
		},
	},
	{
		Name: "getReactions", Method: "GET", Path: "/reactions/list/{actorId}",
		Summary: "List the reactions (comment/event tree) under an actor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Parent (root) actor UUID."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 30)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			{Name: "orderValue", In: InQuery, Type: "string", Enum: []string{"ASC", "DESC"}, Desc: "Sort order by creation time."},
			{Name: "view", In: InQuery, Type: "string", Enum: []string{"tree", "flat", "thread"}, Desc: "Result shape: nested tree, flat list, or thread."},
			{Name: "parentId", In: InQuery, Type: "string", Desc: "Only the replies under this reaction id."},
			{Name: "reactionName", In: InQuery, Type: "string", Desc: "Filter by reaction type/name."},
			{Name: "withChildrenCount", In: InQuery, Type: "boolean", Desc: "Include the reply count per reaction."},
			{Name: "from", In: InQuery, Type: "number", Desc: "Created-at lower bound (unixtime)."},
			{Name: "to", In: InQuery, Type: "number", Desc: "Created-at upper bound (unixtime)."},
		},
	},
	{
		Name: "getReactionsStats", Method: "GET", Path: "/reactions/stats/{actorId}",
		Summary: "Get aggregate reaction statistics for an actor (counts by type, etc.).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Parent (root) actor UUID."},
		},
	},
	{
		Name: "markReactionsRead", Method: "POST", Path: "/reactions/read/{actorId}",
		Summary: "Mark how many of an actor's reactions the current user has read (clears the unread badge).",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Parent (root) actor UUID."},
			{Name: "count", In: InBody, Type: "number", Desc: "Number of reactions marked read."},
			{Name: "time", In: InBody, Type: "number", Desc: "Read-up-to timestamp (unixtime)."},
		},
	},
	{
		Name: "getPinnedReactions", Method: "GET", Path: "/reactions/pinned/{actorId}",
		Summary: "List the pinned reactions of an actor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Parent (root) actor UUID."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (max 30)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset."},
			{Name: "orderValue", In: InQuery, Type: "string", Enum: []string{"ASC", "DESC"}, Desc: "Sort order by creation time."},
		},
	},
	{
		Name: "togglePinnedReaction", Method: "PUT", Path: "/reactions/pinned/{actorId}",
		Summary: "Pin or unpin a reaction. Identify it by reactionId; actorId is its parent (root) actor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "Parent (root) actor UUID the reaction belongs to."},
			{Name: "reactionId", In: InBody, Wire: "actorId", Type: "string", Required: true, Desc: "UUID of the reaction to pin/unpin."},
			{Name: "pinned", In: InBody, Type: "boolean", Required: true, Desc: "true to pin, false to unpin."},
		},
	},
}
