package tools

// publicLinkOps — manage a **public join link** for an actor, used to let people
// connect to a meeting / SIP room without a Simulator account. The link maps to
// the actor and resolves to the public meeting page `<host>/m/<hash>`. Backed by
// a TTL'd store, so a link expires; generate again to refresh.
//
// These are distinct from graph "links" (edges): see createLink / getActorLinks
// for actor-to-actor edges. Here a "link" is a shareable URL onto one actor.
var publicLinkOps = []Operation{
	{
		Name: "generatePublicLink", Method: "POST", Path: "/public_link/{actorId}",
		Summary: "Create (or refresh) a public join link for an actor — e.g. to let external people join a " +
			"meeting / SIP room without logging in. Returns { hash, url } where url is `<host>/m/<hash>`. " +
			"The link is temporary (expires per ttl); call again to issue a fresh one. The actor is usually a " +
			"meeting (Events actor with scheduleMeeting). Requires modify access on the actor.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "UUID of the actor to expose a public join link for."},
			{Name: "waitList", In: InBody, Type: "boolean", Required: true, Desc: "Whether joiners wait for admission (waiting room) before entering — true = admit manually, false = join directly."},
			{Name: "dueDate", In: InBody, Type: "number", Desc: "Optional unix-seconds time after which the link should not be used (e.g. the meeting's scheduled end)."},
			{Name: "ttl", In: InBody, Type: "number", Desc: "Optional link lifetime in seconds (60–86400). Defaults to the server's default if omitted."},
			{Name: "hash", In: InBody, Type: "string", Desc: "Optional custom 8-char link id (alphanumeric). Omit to auto-generate; pass to reuse a known short link."},
		},
	},
	{
		Name: "getPublicLink", Method: "GET", Path: "/public_link/{actorId}",
		Summary: "Get the active public join link for an actor (the { hash, url } if one exists, or null if there " +
			"is none / it expired). Use to show or re-share the current meeting link. Requires view access.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "UUID of the actor whose public link to fetch."},
		},
	},
	{
		Name: "revokePublicLink", Method: "DELETE", Path: "/public_link/{actorId}",
		Summary: "Revoke (delete) the public join link for an actor, so the `/m/<hash>` URL stops working. " +
			"Use to cut off access after a meeting. Requires modify access.",
		Params: []Param{
			{Name: "actorId", In: InPath, Type: "string", Required: true, Desc: "UUID of the actor whose public link to revoke."},
		},
	},
}
