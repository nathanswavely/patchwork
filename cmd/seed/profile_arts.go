package main

// ---------------------------------------------------------------------------
// The seed dataset — a fictional Lancaster, PA arts scene (ADR 009: real
// places, fictional actors). 30 users, ~29 claimed patches (including 4
// invite-only bands with curated overlap), 8 unclaimed fictional venues/
// institutions, 2 pending submissions, 40+ events, 12 proposals, 9
// governance docs. See profileData for the shared seeding machinery this
// plugs into.
// ---------------------------------------------------------------------------

func artsProfile() profileData {
	return profileData{
		users:              artsUsers,
		tags:               artsTags,
		nodes:              artsNodes,
		events:             artsEvents,
		proposals:          artsProposals,
		govDocs:            artsGovDocs,
		notifications:      artsNotifications,
		unclaimed:          artsUnclaimed,
		pendingSubmissions: artsPendingSubmissions,
		extraMemberships:   artsExtraMemberships,
	}
}

var artsUsers = []userDef{
	{"admin@localhost", "patchwork-admin", "Patchwork Admin", "Platform administrator for the Lancaster Patchwork instance.", "admin", 365},
	{"elena.voss@example.com", "elena-voss", "Elena Voss", "Mixed-media artist and community organizer. Co-founded the First Friday Collective in 2016.", "member", 340},
	{"marcus.reed@example.com", "marcus-reed", "Marcus Reed", "Jazz guitarist and music educator. Plays weekly at The Selvage and teaches at a local college.", "member", 330},
	{"sophia.chen@example.com", "sophia-chen", "Sophia Chen", "Gallery curator and art historian. Runs contemporary exhibitions on Gallery Row.", "member", 320},
	{"devon.watts@example.com", "devon-watts", "Devon Watts", "Muralist and public art advocate. Has painted 14 murals across Lancaster's neighborhoods.", "member", 310},
	{"clara.hoffman@example.com", "clara-hoffman", "Clara Hoffman", "Printmaker and letterpress enthusiast. Teaches workshops at Brayer & Press.", "member", 300},
	{"james.okafor@example.com", "james-okafor", "James Okafor", "Ceramic artist and co-op founder. Trained in Japanese raku technique.", "member", 290},
	{"lily.martinez@example.com", "lily-martinez", "Lily Martinez", "Documentary filmmaker telling stories of Lancaster's immigrant communities.", "member", 280},
	{"noah.kim@example.com", "noah-kim", "Noah Kim", "Sound engineer and community radio DJ. Produces the 'Lancaster Frequencies' show.", "member", 270},
	{"ava.patel@example.com", "ava-patel", "Ava Patel", "Contemporary dancer and choreographer. Artistic director of Floorwork Dance Collective.", "member", 260},
	{"ben.sawyer@example.com", "ben-sawyer", "Ben Sawyer", "Venue owner and live music promoter. Keeps Lancaster's independent music scene alive.", "member", 250},
	{"mia.johnson@example.com", "mia-johnson", "Mia Johnson", "Textile artist exploring traditional quilting techniques with modern aesthetics.", "member", 240},
	{"carlos.rivera@example.com", "carlos-rivera", "Carlos Rivera", "Street photographer documenting Lancaster's evolving neighborhoods and people.", "member", 230},
	{"hannah.wright@example.com", "hannah-wright", "Hannah Wright", "Theater director and playwright. Believes in theater as community dialogue.", "member", 220},
	{"omar.hassan@example.com", "omar-hassan", "Omar Hassan", "Sculptor working in reclaimed materials. Studio in the Warehouse Arts Collective.", "member", 210},
	{"rachel.green@example.com", "rachel-green", "Rachel Green", "Arts educator running after-school programs for underserved youth.", "member", 200},
	{"david.park@example.com", "david-park", "David Park", "Graphic designer and zine maker. Organizes an annual zine fest.", "member", 190},
	{"nina.scott@example.com", "nina-scott", "Nina Scott", "Poet and spoken word performer. Hosts open mic nights across the city.", "member", 180},
	{"theo.baker@example.com", "theo-baker", "Theo Baker", "Urban farmer and food justice advocate. Runs the SoWe community garden.", "member", 170},
	{"maya.thompson@example.com", "maya-thompson", "Maya Thompson", "Yoga instructor and holistic wellness practitioner. Teaches donation-based classes.", "member", 160},
	{"sam.nguyen@example.com", "sam-nguyen", "Sam Nguyen", "Full-stack developer and civic tech volunteer. Built tools for local mutual aid networks.", "member", 150},
	{"lucia.santos@example.com", "lucia-santos", "Lucia Santos", "Community organizer for Lancaster's Latinx community. Runs bilingual literacy programs.", "member", 140},
	{"jake.miller@example.com", "jake-miller", "Jake Miller", "Bicycle mechanic and co-op volunteer. Believes in bikes as liberation technology.", "member", 130},
	{"kira.yamamoto@example.com", "kira-yamamoto", "Kira Yamamoto", "Bookbinder and paper artist. Runs workshops at the Tinker's Damn.", "member", 120},
	{"deshawn.carter@example.com", "deshawn-carter", "DeShawn Carter", "Hip-hop producer and youth mentor. Founded the Low End Lab.", "member", 110},
	{"isabel.ruiz@example.com", "isabel-ruiz", "Isabel Ruiz", "Baker and fermentation enthusiast. Teaches sourdough and kimchi at Market Row.", "member", 100},
	{"alex.foster@example.com", "alex-foster", "Alex Foster", "Improv comedian and theater teacher. Runs drop-in improv jams.", "member", 90},
	{"priya.shah@example.com", "priya-shah", "Priya Shah", "Environmental educator. Leads nature walks and foraging workshops in Lancaster County.", "member", 80},
	{"tom.hennessy@example.com", "tom-hennessy", "Tom Hennessy", "Retired librarian and local history buff. Volunteers with a local history archive.", "member", 70},
	{"zoe.washington@example.com", "zoe-washington", "Zoe Washington", "Screen printer and activist. Makes posters for every rally and benefit show in town.", "member", 60},
}

var artsTags = []string{
	"music", "visual-arts", "film", "ceramics", "printmaking",
	"community", "venue", "gallery", "radio", "theater", "dance",
	"food", "craft", "wellness", "education", "tech", "literary",
	"sports", "punk", "folk", "jazz",
}

var artsNodes = []nodeDef{
	{
		name: "Lancaster Arts District", slug: "lancaster-arts-district",
		description: "A coalition of galleries, studios, and performance spaces in downtown Lancaster. Coordinates programming, advocates for artist-friendly policy, and runs the annual Art City festival.",
		ownerIdx:    1,
		tags:        []string{"visual-arts", "community", "gallery"},
		lat:         40.0380, lng: -76.3055,
		membershipPolicy: "open",
		address:          "Downtown Lancaster, PA",
		palette:          "liberalAnimation",
		block:            "logCabin",
		website:          "https://artsdistrict.example",
		links: []nodeLink{
			{URL: "https://instagram.example/artsdistrict", Label: "Instagram"},
			{URL: "https://artsdistrict.example/first-friday", Label: "First Friday Guide"},
		},
		followerPerms: &followerPerms{Events: true, Proposals: true, Charters: true, Members: true},
	},
	{
		name: "Gallery Row", slug: "gallery-row",
		description: "Artist-run galleries along North Prince Street. Open every First Friday with rotating exhibitions, artist talks, and receptions. Currently 8 member galleries.",
		ownerIdx:    3,
		tags:        []string{"visual-arts", "gallery"},
		lat:         40.0392, lng: -76.3050,
		membershipPolicy: "approval_required",
		address:          "N Prince St, Lancaster, PA 17603",
		website:          "https://galleryrow.example",
		links: []nodeLink{
			{URL: "https://instagram.example/galleryrow", Label: "Instagram"},
		},
	},
	{
		name: "First Friday Collective", slug: "first-friday-collective",
		description: "Organizes a monthly First Friday art walk across downtown Lancaster. Coordinates 30+ venues, prints maps, and promotes emerging artists alongside established ones.",
		ownerIdx:    1,
		tags:        []string{"visual-arts", "community"},
		lat:         40.0382, lng: -76.3048,
		membershipPolicy: "open",
		address:          "Downtown Lancaster, PA 17603",
		followerPerms:    &followerPerms{Events: true, Proposals: true, Charters: false, Members: false},
	},
	{
		name: "The Selvage", slug: "the-selvage",
		description: "Three-story live music venue in a historic building on King Street. Features an Irish pub, rooftop garden, underground music hall, and art gallery. Hosts 200+ shows per year.",
		ownerIdx:    10,
		tags:        []string{"music", "venue"},
		lat:         40.0378, lng: -76.3062,
		membershipPolicy: "open",
		address:          "E King St, Lancaster, PA 17602",
		// The quilting-named venue wears a drafted block: an Economy
		// (square-in-a-square) drafted on a 4x4 grid — exercises the
		// drafted-block path end to end (docs/adr/029).
		draftBlock: `{"grid":4,"seams":[[8,0,16,8],[16,8,8,16],[8,16,0,8],[0,8,8,0]],"colors":{"0,0":[1],"0,3":[1],"3,0":[1],"3,3":[1],"1,1":[0],"1,2":[0],"2,1":[0],"2,2":[0],"0,1":[1,0],"0,2":[1,0],"1,0":[1,0],"1,3":[1,0],"2,0":[0,1],"2,3":[0,1],"3,1":[0,1],"3,2":[0,1]}}`,
		bundle:     []string{"#DA0956", "#9FC3DA"},
		website:    "https://theselvage.example",
		links: []nodeLink{
			{URL: "https://instagram.example/theselvage", Label: "Instagram"},
			{URL: "https://theselvage.example/calendar", Label: "Show Calendar"},
			{URL: "https://facebook.example/theselvage", Label: "Facebook"},
		},
	},
	{
		name: "Brayer & Press Guild", slug: "brayer-and-press-guild",
		description: "Shared studio for printmaking arts in a converted row house. Letterpress, screen printing, linocut, etching, and risograph. Open workshops every Saturday.",
		ownerIdx:    5,
		tags:        []string{"printmaking", "visual-arts", "craft"},
		lat:         40.0395, lng: -76.3015,
		membershipPolicy: "approval_required",
		address:          "N Prince St, Lancaster, PA 17603",
	},
	{
		name: "Flicker & Still", slug: "flicker-still",
		description: "Independent cinema, craft distillery, and performance space on North Water Street. Screens indie films, hosts live music, spoken word, and pours house-made spirits.",
		ownerIdx:    7,
		tags:        []string{"film", "venue", "music"},
		lat:         40.0418, lng: -76.3007,
		membershipPolicy: "open",
		address:          "N Water St, Lancaster, PA 17603",
		website:          "https://flickerandstill.example",
		links: []nodeLink{
			{URL: "https://instagram.example/flickerandstill", Label: "Instagram"},
			{URL: "https://flickerandstill.example/films", Label: "Now Showing"},
		},
	},
	{
		name: "Wallflower Mural Project", slug: "wallflower-mural-project",
		description: "Partners with neighborhoods to design and paint large-scale public murals. 23 murals completed since 2019. Runs a youth apprenticeship each summer.",
		ownerIdx:    4,
		tags:        []string{"visual-arts", "community", "education"},
		lat:         40.0405, lng: -76.3035,
		membershipPolicy: "open",
		address:          "Lancaster, PA",
	},
	{
		name: "Common Ground Ceramics Co-op", slug: "common-ground-ceramics",
		description: "Member-run ceramics studio with shared kilns, wheels, and glaze lab. Hosts raku firings, workshops, and seasonal sales. 35 active members sharing the space.",
		ownerIdx:    6,
		tags:        []string{"ceramics", "community", "craft"},
		lat:         40.0412, lng: -76.2985,
		membershipPolicy: "approval_required",
		address:          "E Chestnut St, Lancaster, PA 17602",
		website:          "https://commongroundceramics.example",
		links: []nodeLink{
			{URL: "https://instagram.example/cgceramics", Label: "Instagram"},
			{URL: "https://commongroundceramics.example/classes", Label: "Class Schedule"},
		},
	},
	{
		name: "Red Rose Community Radio", slug: "red-rose-radio",
		description: "Low-power FM station (99.3) run by 40+ volunteer DJs. Local music, community news, arts programming, and live in-studio sessions. On air since 2021.",
		ownerIdx:    8,
		tags:        []string{"radio", "music", "community"},
		lat:         40.0368, lng: -76.3070,
		membershipPolicy: "open",
		address:          "S Queen St, Lancaster, PA 17603",
		website:          "https://redroseradio.example",
		links: []nodeLink{
			{URL: "https://redroseradio.example/listen", Label: "Listen Live"},
			{URL: "https://instagram.example/redroseradio", Label: "Instagram"},
			{URL: "https://redroseradio.example/schedule", Label: "DJ Schedule"},
		},
	},
	{
		name: "Warehouse Arts Collective", slug: "warehouse-arts-collective",
		description: "Converted tobacco warehouse providing affordable studio space for 18 visual artists, sculptors, and makers. Quarterly open studio weekends draw 500+ visitors.",
		ownerIdx:    14,
		tags:        []string{"visual-arts", "community"},
		lat:         40.0445, lng: -76.2975,
		membershipPolicy: "approval_required",
		address:          "N Plum St, Lancaster, PA 17602",
	},
	{
		name: "Floorwork Dance Collective", slug: "floorwork-dance-collective",
		description: "Home for contemporary dance in Lancaster County. Offers classes, rehearsal space, and produces two showcases per year. Founded by dancers tired of driving to Philly.",
		ownerIdx:    9,
		tags:        []string{"dance", "community"},
		lat:         40.0388, lng: -76.3028,
		membershipPolicy: "open",
		address:          "W King St, Lancaster, PA 17603",
	},
	{
		name: "Orange Street Players", slug: "orange-street-players",
		description: "Volunteer-driven theater producing plays, musicals, and improv. Four mainstage productions plus a summer youth show. Performs in a converted church on Orange Street.",
		ownerIdx:    13,
		tags:        []string{"theater", "community"},
		lat:         40.0372, lng: -76.3042,
		membershipPolicy: "open",
		address:          "E Orange St, Lancaster, PA 17602",
	},
	// Name search-checked 2026-07-24 per the ADR 009 naming rule: "Tinker's
	// Damn" exists only as the idiom, no real organization carries it.
	{
		name: "The Tinker's Damn", slug: "tinkers-damn",
		description: "Community makerspace with woodshop, laser cutter, 3D printers, sewing machines, and electronics bench. Pay-what-you-can memberships. Tool library for the neighborhood.",
		ownerIdx:    20,
		tags:        []string{"craft", "tech", "community"},
		lat:         40.0401, lng: -76.2995,
		membershipPolicy: "open",
		address:          "N Christian St, Lancaster, PA 17602",
		website:          "https://tinkersdamn.example",
		links: []nodeLink{
			{URL: "https://tinkersdamn.example/tools", Label: "Tool Library"},
			{URL: "https://instagram.example/tinkersdamnlanc", Label: "Instagram"},
		},
	},
	{
		name: "SoWe Community Garden", slug: "sowe-garden",
		description: "50-plot community garden in the southwest neighborhood. Free plots for residents, composting program, seed library, and monthly potlucks. Run entirely by volunteers.",
		ownerIdx:    18,
		tags:        []string{"food", "community"},
		lat:         40.0342, lng: -76.3090,
		membershipPolicy: "open",
		address:          "SW Lancaster, PA 17603",
	},
	{
		name: "The Freewheelery", slug: "freewheelery",
		description: "Earn-a-bike workshop where anyone can learn to fix and build bicycles. Free tools, donated parts, and volunteer mechanics. Open wrenching hours Tuesday and Thursday evenings.",
		ownerIdx:    22,
		tags:        []string{"community", "sports"},
		lat:         40.0355, lng: -76.3065,
		membershipPolicy: "open",
		address:          "S Duke St, Lancaster, PA 17602",
	},
	{
		name: "Low End Lab", slug: "low-end-lab",
		description: "Free music production studio for youth ages 14-21. Beat making, recording, mixing, and music business. Mentored by working producers. Equipment provided.",
		ownerIdx:    24,
		tags:        []string{"music", "education", "community"},
		lat:         40.0365, lng: -76.3040,
		membershipPolicy: "open",
		address:          "S Queen St, Lancaster, PA 17602",
		website:          "https://lowendlab.example",
		links: []nodeLink{
			{URL: "https://soundcloud.example/lowendlab", Label: "SoundCloud"},
			{URL: "https://instagram.example/lowendlab", Label: "Instagram"},
		},
	},
	{
		name: "Market Row Vendors Collective", slug: "market-row-vendors",
		description: "Collective of vendors at the Market Row weekend artisan and farmers market. Coordinates events, shared marketing, and advocates for small food producers.",
		ownerIdx:    25,
		tags:        []string{"food", "community"},
		lat:         40.0384, lng: -76.3060,
		membershipPolicy: "approval_required",
		address:          "N Market St, Lancaster, PA 17603",
		website:          "https://marketrow.example",
		links: []nodeLink{
			{URL: "https://marketrow.example/vendors", Label: "Vendor Directory"},
			{URL: "https://facebook.example/marketrow", Label: "Facebook"},
		},
	},
	{
		name: "El Telar", slug: "el-telar",
		description: "Cultural center serving Lancaster's Latinx community. Bilingual literacy programs, legal aid referrals, cultural celebrations, and a community kitchen for cooking classes.",
		ownerIdx:    21,
		tags:        []string{"community", "education", "food"},
		lat:         40.0358, lng: -76.3025,
		membershipPolicy: "open",
		address:          "S Prince St, Lancaster, PA 17603",
	},
	{
		name: "Longhand Writers Guild", slug: "longhand-writers-guild",
		description: "Peer workshop for fiction, nonfiction, and poetry writers. Monthly critique groups, annual anthology, and a reading series at local bookshops. All genres, all levels.",
		ownerIdx:    17,
		tags:        []string{"literary", "community"},
		lat:         40.0390, lng: -76.3072,
		membershipPolicy: "open",
		address:          "Lancaster, PA",
	},
	{
		name: "Yoga in the Park", slug: "yoga-in-the-park",
		description: "Donation-based outdoor yoga classes in Musser Park, Buchanan Park, and Long's Park. All levels welcome. Bring your own mat. Rain moves us to the pavilion.",
		ownerIdx:    19,
		tags:        []string{"wellness", "community"},
		lat:         40.0410, lng: -76.3080,
		membershipPolicy: "open",
		address:          "Lancaster City Parks",
	},
	{
		name: "Commits & Coffee", slug: "code-and-coffee",
		description: "Weekly meetup for developers, designers, and tech-curious folks. Saturday mornings at rotating coffee shops. Bring a laptop or just come to learn. No experience required.",
		ownerIdx:    20,
		tags:        []string{"tech", "community"},
		lat:         40.0387, lng: -76.3045,
		membershipPolicy: "open",
		address:          "Various coffee shops, Lancaster",
		links: []nodeLink{
			{URL: "https://meetup.example/commits-and-coffee", Label: "Meetup"},
			{URL: "https://github.example/commits-and-coffee", Label: "GitHub"},
		},
	},
	{
		name: "Half-Fold Zine Library", slug: "half-fold-zine-library",
		description: "Free lending library of zines, small press publications, and artist books. Submit your own work to the collection. Monthly zine-making workshops. Self-published since 2018.",
		ownerIdx:    16,
		tags:        []string{"literary", "visual-arts", "craft"},
		lat:         40.0398, lng: -76.3010,
		membershipPolicy: "open",
		address:          "N Water St, Lancaster, PA 17603",
	},
	{
		name: "Screen Door Mutual Aid", slug: "screen-door-mutual-aid",
		description: "Neighbor-to-neighbor support network. Coordinates grocery delivery, rides, childcare swaps, and emergency funds. Not charity — solidarity. Run on Signal and spreadsheets.",
		ownerIdx:    21,
		tags:        []string{"community"},
		lat:         40.0375, lng: -76.3050,
		membershipPolicy: "open",
		address:          "Lancaster, PA",
		links: []nodeLink{
			{URL: "https://linktree.example/screendoormutualaid", Label: "Linktree"},
			{URL: "https://instagram.example/screendoormutualaid", Label: "Instagram"},
		},
	},
	{
		name: "The Sculpture Yard", slug: "sculpture-yard",
		description: "Outdoor sculpture garden and metalworking space behind the Warehouse Arts Collective. Rotating installations, welding workshops, and a yearly iron pour.",
		ownerIdx:    14,
		tags:        []string{"visual-arts", "craft"},
		lat:         40.0447, lng: -76.2972,
		membershipPolicy: "approval_required",
		address:          "N Plum St, Lancaster, PA 17602",
	},
	{
		name: "The Crock Society", slug: "fermentation-collective",
		description: "Community of home fermenters sharing cultures, knowledge, and taste tests. Sourdough, kombucha, kimchi, miso, hot sauce. Monthly swaps at Market Row.",
		ownerIdx:    25,
		tags:        []string{"food", "community", "craft"},
		lat:         40.0383, lng: -76.3058,
		membershipPolicy: "open",
		address:          "Lancaster, PA",
	},

	// -------------------------------------------------------------------
	// Bands — invite-only, all-admin membership (CLAUDE.md: "A band:
	// invite-only, all members are admins, no proposals or governance docs
	// needed"). Rosters are wired in artsExtraMemberships from existing
	// users, so the quilt's threads connect bands to the studios, co-ops,
	// and venues their members already belong to.
	// -------------------------------------------------------------------
	{
		name: "Mill 72", slug: "mill-72",
		description: "Jazz trio playing standards and original compositions, named for the old mill address where they still rehearse every Tuesday.",
		ownerIdx:    2,
		tags:        []string{"music", "jazz"},
		lat:         40.0402, lng: -76.2960,
		membershipPolicy: "invite_only",
		address:          "Rehearsal space, Lancaster, PA 17602",
	},
	{
		name: "Static Season", slug: "static-season",
		description: "Fast, loud, and DIY. Static Season plays basements, benefit shows, and whatever all-ages space will have them. Posters screen-printed by the singer.",
		ownerIdx:    29,
		tags:        []string{"music", "punk"},
		lat:         40.0362, lng: -76.3095,
		membershipPolicy: "invite_only",
		address:          "Basement practice space, Lancaster, PA 17603",
	},
	{
		name: "Chestnut Hollow", slug: "chestnut-hollow",
		description: "Indie folk duo writing quiet, careful songs about Lancaster County — barns, back roads, and the garden at closing time.",
		ownerIdx:    17,
		tags:        []string{"music", "folk"},
		lat:         40.0392, lng: -76.3005,
		membershipPolicy: "invite_only",
		address:          "Lancaster, PA 17602",
	},
	{
		name: "Conestoga Drift", slug: "conestoga-drift",
		description: "Shoegaze three-piece drowning vocals in reverb out of a Cabbage Hill basement. One EP, one seven-inch, and a slow-building following.",
		ownerIdx:    23,
		tags:        []string{"music"},
		lat:         40.0355, lng: -76.3112,
		membershipPolicy: "invite_only",
		address:          "Cabbage Hill, Lancaster, PA 17603",
	},
}

// ---------------------------------------------------------------------------
// Curated memberships — band rosters (100% admin, per the band governance
// shape) plus a few cross-memberships that make the inferred threads tell a
// story: the punk band threads to the print guild, bike co-op, and zine
// library through its members; the jazz trio to the radio station and the
// beats lab; the folk duo to the writers guild and the garden.
// ---------------------------------------------------------------------------

var artsExtraMemberships = []extraMembershipDef{
	// Mill 72 (owner 2, Marcus Reed) — jazz trio.
	{8, "mill-72", "admin"},
	{24, "mill-72", "admin"},

	// Static Season (owner 29, Zoe Washington) — punk trio.
	{22, "static-season", "admin"},
	{16, "static-season", "admin"},

	// Chestnut Hollow (owner 17, Nina Scott) — folk duo.
	{18, "chestnut-hollow", "admin"},

	// Conestoga Drift (owner 23, Kira Yamamoto) — shoegaze trio.
	{20, "conestoga-drift", "admin"},
	{14, "conestoga-drift", "admin"},

	// Cross-memberships that thicken the threads around the bands.
	{29, "brayer-and-press-guild", "member"},
	{23, "tinkers-damn", "member"},
	{2, "the-selvage", "follower"},
	{24, "red-rose-radio", "follower"},
}

var artsEvents = []eventDef{
	// First Friday (recurring)
	{"First Friday Gallery Walk", "Monthly art walk through downtown Lancaster galleries. Meet the artists, enjoy refreshments, discover new work. 30+ participating venues.", "first-friday-collective", "Downtown Lancaster", -56, 4},
	{"First Friday Gallery Walk", "Spring exhibitions and live music at participating galleries. Special printmaking demos at Brayer & Press.", "first-friday-collective", "Downtown Lancaster", -28, 4},
	{"First Friday Gallery Walk", "April edition with a focus on local photography and mixed media. New gallery openings on Prince Street.", "first-friday-collective", "Downtown Lancaster", 7, 4},
	{"First Friday Gallery Walk", "Summer kickoff edition. Extended hours until 10pm. Food trucks on Queen Street.", "first-friday-collective", "Downtown Lancaster", 35, 4},

	// Printmakers Guild
	{"Printmaking Workshop: Intro to Linocut", "Learn the basics of linocut printing. All materials provided. No experience necessary. Leave with your own print.", "brayer-and-press-guild", "Brayer & Press Guild", -21, 3},
	{"Letterpress Open Studio", "Drop in and explore our vintage Vandercook and Chandler & Price presses. Print a broadside to take home.", "brayer-and-press-guild", "Brayer & Press Guild", 14, 3},
	{"Risograph Print Party", "Bring your artwork and we'll run it through our Riso. Learn color separation and registration. $5 materials fee.", "brayer-and-press-guild", "Brayer & Press Guild", 28, 3},

	// The Selvage
	{"Open Mic Night", "Weekly open mic for musicians, poets, and comedians. Sign up starts at 7pm. House PA and backline provided.", "the-selvage", "The Selvage, E King St", -14, 3},
	{"Mill 72 Live in the Underground", "Original jazz compositions and standards in the underground hall. $10 cover, $5 students.", "the-selvage", "The Selvage Underground", -7, 2},
	{"Live Music: Lancaster Roots Revival", "Americana and folk showcase with three regional acts. Rooftop show, weather permitting.", "the-selvage", "The Selvage Rooftop", 21, 3},
	{"Irish Session Night", "Traditional Irish session in the pub. Bring your fiddle, tin whistle, or bodhrán. Listeners welcome.", "the-selvage", "The Selvage Pub", 3, 3},

	// Mural Arts
	{"Community Mural Planning Session", "Help design the next neighborhood mural. Bring your ideas and sketches. All skill levels welcome.", "wallflower-mural-project", "Southside Rec Center, S Duke St", -10, 2},
	{"Mural Painting Day: Chestnut Street Wall", "Volunteers needed to paint the approved design. Supplies and lunch provided. Wear old clothes.", "wallflower-mural-project", "Chestnut St & Lime St", 10, 6},
	{"Youth Mural Apprenticeship Info Session", "Learn about the summer apprenticeship for ages 15-19. Paid stipend, professional training, public portfolio piece.", "wallflower-mural-project", "Southside Rec Center", 18, 2},

	// Ceramics
	{"Ceramic Raku Firing Day", "Outdoor raku firing event. Bring your bisqueware or use studio pieces. Dramatic results guaranteed. Weather permitting.", "common-ground-ceramics", "Common Ground Ceramics", -18, 5},
	{"Wheel Throwing for Beginners", "Six-week introductory course. Wednesdays 6-8pm. Materials included. $120 members / $160 non-members.", "common-ground-ceramics", "Common Ground Ceramics", 3, 2},
	{"Spring Ceramics Sale", "Annual sale of member-made pottery, sculpture, and functional ware. All proceeds support the co-op.", "common-ground-ceramics", "Common Ground Ceramics", 30, 5},

	// Flicker & Still
	{"Documentary Screening: Local Voices", "New short doc exploring Lancaster's immigrant food traditions. Discussion with filmmaker Lily Martinez to follow.", "flicker-still", "Flicker & Still Cinema", -30, 2},
	{"Indie Film Night: Regional Shorts", "Curated shorts by Mid-Atlantic filmmakers. Q&A with directors. $8 admission includes a craft spirit tasting.", "flicker-still", "Flicker & Still Cinema", 17, 3},
	{"Spoken Word Night", "Featured performer: Nina Scott. Open list follows. Sign up at the bar. Free admission.", "flicker-still", "Flicker & Still Cinema", -3, 2},
	{"Zine Reading & Swap", "Authors read from new zines followed by a free swap table. Bring copies of your work to trade.", "flicker-still", "Flicker & Still Cinema", 25, 2},

	// Radio
	{"Community Radio Fundraiser", "Annual fundraiser for Red Rose Radio. Live bands, food trucks, silent auction. Help us stay on the air.", "red-rose-radio", "Millrace Park, Lancaster", -42, 5},
	{"Radio Workshop: Intro to Podcasting", "Plan, record, and edit a podcast using our studio. Leave with a published episode. No experience needed.", "red-rose-radio", "Red Rose Radio Studio", 24, 3},

	// Dance
	{"Dance Collective Showcase: Spring Movement", "New works by collective members. Two performances. $15 general / $10 students. At Stonegate Hall.", "floorwork-dance-collective", "Stonegate Hall, Lancaster", 28, 2},
	{"Contact Improvisation Jam", "Open jam for movers of all levels. Warm-up at 6pm, open dancing at 6:30. Soft-soled shoes or bare feet.", "floorwork-dance-collective", "W King St Studio", 5, 2},

	// Theater
	{"Improv Night: No Script Required", "Fast-paced improv comedy driven by audience suggestions. $8 at the door. BYOB.", "orange-street-players", "Orange Street Playhouse, E Orange St", -5, 2},
	{"Auditions: Summer One-Act Festival", "Open auditions for three one-act plays. Prepare a 2-minute monologue. All roles available.", "orange-street-players", "Orange Street Playhouse", 12, 3},

	// Warehouse / Sculpture Yard
	{"Open Studio Weekend", "Tour 18 artist studios. Works for sale, live demos, refreshments. Free admission.", "warehouse-arts-collective", "Warehouse Arts Collective", -35, 8},
	{"Iron Pour", "Annual iron pour in the Sculpture Yard. Watch molten metal become art. Bring the kids. Hot dogs provided.", "sculpture-yard", "Sculpture Yard, N Plum St", 22, 4},

	// The Tinker's Damn
	{"Intro to Laser Cutting", "Learn to design and cut with our Glowforge. Bring a simple design or use our templates. Materials provided.", "tinkers-damn", "The Tinker's Damn", 8, 2},
	{"Fix-It Clinic", "Bring your broken stuff — toasters, lamps, bikes, clothing. Volunteer fixers will help you repair instead of replace.", "tinkers-damn", "The Tinker's Damn", 15, 3},

	// Garden
	{"Spring Plot Assignments", "Annual plot lottery and orientation for the growing season. Bring proof of SW Lancaster residency. Free.", "sowe-garden", "SoWe Community Garden", 4, 2},
	{"Composting Workshop", "Learn to build and maintain a backyard compost bin. Take home a starter kit. Presented by Theo Baker.", "sowe-garden", "SoWe Community Garden", 20, 2},

	// Bike Co-op
	{"Open Wrench Night", "Fix your bike with our tools and volunteer help. Tubes and basic parts available by donation.", "freewheelery", "The Freewheelery", -7, 3},
	{"Earn-a-Bike Orientation", "Start the earn-a-bike program: volunteer 10 hours and take home a refurbished bicycle. Orientation required.", "freewheelery", "The Freewheelery", 6, 2},

	// Beats Lab
	{"Beat Battle", "16 producers, one night. Bring your best beat on a USB stick. Audience votes. Prizes from local music shops.", "low-end-lab", "Low End Lab", 9, 3},
	{"Intro to Ableton Live", "Free workshop for youth. Learn the basics of digital music production. Laptops provided.", "low-end-lab", "Low End Lab", 16, 3},

	// Raíces
	{"Noche de Poesía", "Bilingual poetry night. Read in English, Spanish, or both. Open mic after featured readers. Free. Refreshments.", "el-telar", "El Telar", 11, 2},
	{"Community Kitchen: Pupusas", "Learn to make pupusas with Doña Carmen. All ingredients provided. Donation suggested. Space limited to 15.", "el-telar", "El Telar", 19, 3},

	// Writers Guild
	{"Monthly Critique Group: Fiction", "Bring 3,000 words of fiction. Read, discuss, improve. Constructive feedback only. All levels welcome.", "longhand-writers-guild", "Inkhorn Books, W King St", -12, 2},
	{"Poetry Open Mic", "Read your work or just listen. Hosted by Nina Scott. At the Linotype on Queen Street.", "longhand-writers-guild", "The Linotype", 13, 2},

	// Yoga
	{"Sunset Yoga in Musser Park", "Donation-based vinyasa flow. All levels. Bring your own mat. Rain moves us to the pavilion.", "yoga-in-the-park", "Musser Park, Lancaster", 2, 1},

	// Commits & Coffee
	{"Commits & Coffee: Saturday Session", "Bring a laptop or just come to learn. This week: intro to web scraping with Python.", "code-and-coffee", "The Percolator, S Duke St", 1, 3},

	// Zine Library
	{"Zine Making Workshop", "Learn to fold, print, and bind a one-sheet zine. All supplies provided. Take home 20 copies.", "half-fold-zine-library", "Half-Fold Zine Library", 10, 2},

	// Mutual Aid
	{"Mutual Aid Potluck & Planning", "Monthly gathering to coordinate rides, groceries, and emergency support. Bring a dish. Everyone welcome.", "screen-door-mutual-aid", "Rodney Park Pavilion", 7, 3},

	// Fermentation Collective
	{"Sourdough Starter Swap", "Bring your starter, take someone else's. Compare notes on hydration, flour blends, and feeding schedules.", "fermentation-collective", "Market Row, Lancaster", 14, 2},

	// Arts District
	{"Youth Art Workshop: Zine Making", "Free workshop for teens at the public library. Design, print, and bind your own zine. Materials provided.", "lancaster-arts-district", "Vine Street Free Library", 8, 3},
	{"Artist Talk: Public Art and Community", "Devon Watts on the role of murals in neighborhood identity. Gallery Row, 7pm. Free.", "gallery-row", "Gallery Row, Prince St", 15, 2},

	// Bands
	{"Mill 72 Rehearsal / Listening Session", "Working rehearsal, followed by a listening session of new material. Drop in quietly.", "mill-72", "Rehearsal space, Lancaster", 2, 2},
	{"Static Season Benefit Show", "All-ages benefit for the community fridge. $5 suggested, nobody turned away.", "static-season", "Basement show, SW Lancaster", 9, 3},
	{"Chestnut Hollow Songwriting Circle", "Informal songwriting circle hosted by the duo. Bring a work-in-progress.", "chestnut-hollow", "Living room show, Lancaster", 27, 2},
	{"Conestoga Drift Rehearsal Open House", "Rare open rehearsal — come watch the new record take shape.", "conestoga-drift", "Practice space, Cabbage Hill", 16, 2},
}

var artsProposals = []proposalDef{
	{"Add weekend open studio hours", "Open the studio on Saturdays 10am-4pm. Requires a volunteer coordinator each session. Trial for 3 months.", "common-ground-ceramics", "approved", "action", 45, 168},
	{"Adopt shared equipment lending policy", "Formal lending program for tools between member orgs. Insurance requirements and checkout tracking included.", "warehouse-arts-collective", "approved", "action", 60, 168},
	{"Partner with Food Co-op for event catering", "Locally sourced catering for gallery openings at a 15% discount. Trial partnership for 6 months.", "first-friday-collective", "approved", "action", 30, 120},
	{"Add land acknowledgment to community lining", "Recognize the Susquehannock people in our founding document. Drafted in consultation with local Indigenous educators.", "lancaster-arts-district", "open", "amendment", 5, 168},
	{"Create youth mentorship program", "Pair experienced artists with high school students for semester-long mentorship. 2 hours/week commitment.", "lancaster-arts-district", "open", "action", 3, 240},
	{"Increase monthly dues by $10", "Cover rising utility costs and kiln maintenance. Raise from $40 to $50/month, effective next quarter.", "common-ground-ceramics", "rejected", "action", 50, 168},
	{"Install soundproofing in rehearsal space", "Acoustic panels to reduce noise complaints from neighbors. Three quotes obtained, lowest is $2,400.", "floorwork-dance-collective", "open", "action", 7, 168},
	{"Add anti-harassment policy to community lining", "Explicit policy covering events, workshops, and online spaces. Includes reporting procedures and consequences.", "lancaster-arts-district", "open", "amendment", 2, 336},
	{"Transition studio to solar power", "Install panels on the warehouse roof. Three vendor quotes pending. Estimated 7-year payback.", "warehouse-arts-collective", "withdrawn", "action", 25, 168},
	{"Open tool library to non-members", "Allow neighborhood residents to borrow hand tools with a $20/year library card. Power tools remain member-only.", "tinkers-damn", "open", "action", 4, 168},
	{"Start a community fridge program", "Install a community fridge outside the garden shed. Stocked by volunteers and local farms. 24/7 access.", "sowe-garden", "approved", "action", 35, 120},
	{"Establish code of conduct for online spaces", "Formal guidelines for our Signal group and social media. Moderation process and appeals.", "screen-door-mutual-aid", "open", "amendment", 8, 240},
}

var artsGovDocs = []govDocDef{
	{"lancaster-arts-district", "Community Lining", "We are artists, makers, and supporters building an arts scene in Lancaster, PA with room for everyone.\n\nWe stand against all forms of discrimination including racism, sexism, homophobia, transphobia, and ableism. Our spaces are for everyone.\n\nMembers agree to treat one another with respect, resolve conflicts through dialogue, and uplift emerging voices alongside established ones."},
	{"common-ground-ceramics", "Studio Usage Guidelines", "Studio hours: Monday-Friday 8am-10pm, Saturday-Sunday 10am-6pm.\n\nAll members must complete safety orientation before using kilns or wheels. Clean your workspace after each session. Shared glazes are for members only; label personal supplies clearly. Kiln scheduling is first-come, first-served via the sign-up sheet. Bisque loads fire Tuesdays and Fridays."},
	{"warehouse-arts-collective", "Equipment Lending Policy", "Members may borrow shared tools for up to 7 days. Complete the checkout log and return items clean and working. Report damage immediately. Borrowers are responsible for replacement costs. Power tools require safety certification. No lending to non-members without board approval."},
	{"orange-street-players", "Event Hosting Checklist", "Before: Reserve space 2+ weeks out. Confirm tech needs. Arrange volunteer house manager. Submit event description for socials.\n\nDay-of: Arrive 1 hour early. Check fire exits. Set out donation box.\n\nAfter: Reset furniture. Take out trash/recycling. Lock up and set alarm."},
	{"lancaster-arts-district", "First Friday Participation Agreement", "Participating venues agree to: open by 5pm, display official map, maintain welcoming environment, report attendance monthly, contribute $25/month to shared marketing."},
	{"brayer-and-press-guild", "Studio Membership Terms", "Monthly membership includes: 24/7 access, use of all presses, shared ink and paper, storage shelf. Complete orientation on each press before unsupervised use. Personal editions under 50; commercial runs require board notification."},
	{"tinkers-damn", "Tool Library Rules", "Return tools within 7 days, clean and functional. Report any damage. No modifications to shared equipment. Consumables (sandpaper, drill bits) are replenished monthly — don't hoard. If you break it, tell us. Accidents happen; dishonesty gets your card revoked."},
	{"sowe-garden", "Garden Plot Agreement", "One plot per household. Water your plot at least twice weekly or arrange a buddy. No pesticides. Harvest only from your plot. Donate excess to the community fridge. Clean up by November 1. Plots not planted by June 1 will be reassigned."},
	{"screen-door-mutual-aid", "Solidarity Principles", "We are neighbors, not service providers. We give and receive — everyone has something to offer. No means-testing. No judgement. Confidentiality is sacred. We coordinate, not control. If you need help, ask. If you can help, offer. Mutual aid is not charity — it is solidarity."},
}

var artsNotifications = []notifDef{
	{1, "membership_approved", "Welcome to Gallery Row", "Your membership request for Gallery Row has been approved.", "/nodes/gallery-row", true},
	{2, "new_event", "New event: Jazz Quartet", "A new event has been posted in The Selvage.", "/events", true},
	{3, "proposal_result", "Proposal approved: Shared equipment lending", "The proposal to adopt a shared equipment lending policy has passed.", "/nodes/warehouse-arts-collective/proposals", true},
	{5, "membership_approved", "Welcome to Common Ground Ceramics", "Your membership request has been approved.", "/nodes/common-ground-ceramics", false},
	{6, "proposal_created", "New proposal: Weekend open studio hours", "A new proposal has been created in Common Ground Ceramics.", "/nodes/common-ground-ceramics/proposals", false},
	{4, "new_member", "New member joined Wallflower Murals", "Rachel Green has joined the Wallflower Mural Project.", "/nodes/wallflower-mural-project/members", true},
	{8, "proposal_result", "Proposal rejected: Increase monthly dues", "The proposal to increase monthly dues has been rejected.", "/nodes/common-ground-ceramics/proposals", true},
	{9, "new_event", "New event: Dance Showcase", "A new spring showcase has been announced.", "/events", false},
	{10, "membership_request", "New membership request", "A new member has requested to join Gallery Row.", "/nodes/gallery-row/members", false},
	{14, "proposal_created", "New proposal: Anti-harassment policy", "A new proposal has been created for the Lancaster Arts District.", "/nodes/lancaster-arts-district/proposals", false},
	{1, "new_member", "New member joined First Friday", "David Park has joined the First Friday Collective.", "/nodes/first-friday-collective/members", true},
	{12, "new_event", "New event: Auditions", "Open auditions for the Summer One-Act Festival.", "/events", false},
	{18, "new_event", "New event: Spring Plot Assignments", "Annual plot lottery at SoWe Community Garden.", "/events", false},
	{20, "new_event", "New event: Fix-It Clinic", "Bring your broken stuff to the Tinker's Damn.", "/events", false},
	{24, "new_event", "New event: Beat Battle", "16 producers, one night at the Low End Lab.", "/events", false},
	{22, "membership_approved", "Welcome to the Tinker's Damn", "Your membership at the Tinker's Damn is confirmed.", "/nodes/tinkers-damn", true},
}

var artsUnclaimed = []unclaimedDef{
	{
		name:    "The Cordwainer Theatre",
		desc:    "Historic theatre on Prince Street. Professional productions, touring shows, and education programs.",
		website: "https://cordwainer.example",
		links:   []nodeLink{{URL: "https://instagram.example/cordwainertheatre", Label: "Instagram"}, {URL: "https://cordwainer.example/shows", Label: "Current Season"}},
		tags:    []string{"theater", "venue"},
		lat:     40.0381, lng: -76.3065,
		address: "N Prince St, Lancaster, PA 17603",
	},
	{
		name:    "Granary Hall",
		desc:    "Historic performance venue on North Prince Street. Concerts, comedy, dance, and community events in a renovated vaudeville theatre.",
		website: "https://granaryhall.example",
		links:   []nodeLink{{URL: "https://instagram.example/granaryhall", Label: "Instagram"}},
		tags:    []string{"venue", "music", "dance"},
		lat:     40.0395, lng: -76.3052,
		address: "N Market St, Lancaster, PA 17603",
	},
	{
		name: "Millrace Park",
		desc: "Public green space in the heart of downtown Lancaster. Hosts pop-up markets, food trucks, outdoor performances, and community gatherings.",
		tags: []string{"community"},
		lat:  40.0379, lng: -76.3055,
		address: "Downtown Lancaster, PA 17603",
	},
	{
		name:    "Spark Hall",
		desc:    "Hands-on science museum for all ages. Interactive exhibits, workshops, maker programs, and STEM education for the Lancaster community.",
		website: "https://sparkhall.example",
		links:   []nodeLink{{URL: "https://sparkhall.example/visit", Label: "Plan Your Visit"}, {URL: "https://instagram.example/sparkhall", Label: "Instagram"}},
		tags:    []string{"education", "community", "tech"},
		lat:     40.0410, lng: -76.3015,
		address: "New Holland Ave, Lancaster, PA 17602",
	},
	{
		name:    "Nightjar Park",
		desc:    "Home of the Lancaster Nightjars baseball team. Hosts games, concerts, festivals, and community events.",
		website: "https://nightjars.example",
		links:   []nodeLink{{URL: "https://nightjars.example/schedule", Label: "Game Schedule"}},
		tags:    []string{"sports", "venue", "community"},
		lat:     40.0340, lng: -76.3073,
		address: "N Prince St, Lancaster, PA 17603",
	},
	{
		name:  "The Rowhouse Market",
		desc:  "Indoor artisan market featuring local makers, vintage goods, food vendors, and creative businesses. Open weekends.",
		links: []nodeLink{{URL: "https://instagram.example/rowhousemarket", Label: "Instagram"}},
		tags:  []string{"craft", "food", "community"},
		lat:   40.0373, lng: -76.3038,
		address: "E Lemon St, Lancaster, PA 17602",
	},
	{
		name:    "The Drygoods Museum",
		desc:    "Small museum of regional art in a historic rowhouse. Free admission. Rotating exhibitions.",
		website: "https://drygoodsmuseum.example",
		links:   []nodeLink{{URL: "https://drygoodsmuseum.example/visit", Label: "Visit"}, {URL: "https://instagram.example/drygoodsmuseum", Label: "Instagram"}},
		tags:    []string{"visual-arts", "gallery", "education"},
		lat:     40.0375, lng: -76.3042,
		address: "E King St, Lancaster, PA 17602",
	},
	{
		name:    "Vine Street Free Library",
		desc:    "Community hub for reading, learning, and programs. Free WiFi, maker space, teen programs, ESL classes, and community meeting rooms.",
		website: "https://vinestreetlibrary.example",
		links:   []nodeLink{{URL: "https://vinestreetlibrary.example/events", Label: "Events Calendar"}},
		tags:    []string{"education", "community", "literary"},
		lat:     40.0385, lng: -76.3030,
		address: "S Vine St, Lancaster, PA 17603",
	},
}

var artsPendingSubmissions = []unclaimedDef{
	{
		name: "The Linotype",
		desc: "Restaurant and event space on West King Street. Live jazz, farm-to-table dining, and private event hosting.",
		tags: []string{"venue", "food"},
		lat:  40.0378, lng: -76.3070,
	},
	{
		name: "The Kilowatt",
		desc: "DIY music venue and recording studio in Millersville. All-ages shows, local and touring bands, community space.",
		tags: []string{"music", "venue"},
		lat:  40.0015, lng: -76.3505,
	},
}
