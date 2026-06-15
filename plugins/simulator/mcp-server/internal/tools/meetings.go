package tools

// meetingOps — read-side tools for meetings (Events actors with scheduleMeeting).
// Meeting creation/configuration uses createActor on the Events form; live-call
// operations (join, participants, recording) are platform-managed. The one read
// the agent benefits from is the call transcription.
var meetingOps = []Operation{
	{
		Name: "getTranscription", Method: "GET", Path: "/sip/transcription/{actorId}",
		Summary: "Read a meeting's speech transcription (the spoken-word log of the call on a meeting " +
			"actor). Useful to summarize a call, extract action items, or answer questions about what was " +
			"said. Returns the transcription messages plus total/hasMore for paging. Note: the backend serves " +
			"this for a meeting with a live/recent room — it errors if there is no active call for the actor. " +
			"Requires view access to the actor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "UUID of the meeting actor (Events actor) whose transcription to read."},
			{Name: "limit", In: InQuery, Type: "number", Desc: "Page size (1-100, default server value)."},
			{Name: "offset", In: InQuery, Type: "number", Desc: "Page offset (default 0)."},
			{Name: "orderValue", In: InQuery, Type: "string", Enum: []string{"DESC", "ASC"}, Desc: "Order by time: DESC (newest first, default) or ASC (oldest first — natural reading order)."},
		},
	},
}
