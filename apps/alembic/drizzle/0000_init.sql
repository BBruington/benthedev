CREATE TABLE "brews" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"campaign_id" uuid NOT NULL,
	"mix" jsonb NOT NULL,
	"result" jsonb NOT NULL,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
CREATE TABLE "campaigns" (
	"id" uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
	"gm_user_id" text NOT NULL,
	"viewing_as" text DEFAULT 'player' NOT NULL,
	CONSTRAINT "campaigns_gm_user_id_unique" UNIQUE("gm_user_id")
);
--> statement-breakpoint
CREATE TABLE "codex_entries" (
	"campaign_id" uuid NOT NULL,
	"potion_id" text NOT NULL,
	"mentor" boolean DEFAULT false NOT NULL,
	"formed" boolean DEFAULT false NOT NULL,
	"revealed" boolean DEFAULT false NOT NULL,
	"formula_revealed" boolean DEFAULT false NOT NULL,
	"perfected" boolean DEFAULT false NOT NULL,
	"best_off_by" integer,
	"best_mix" jsonb,
	"note" text,
	CONSTRAINT "codex_entries_campaign_id_potion_id_pk" PRIMARY KEY("campaign_id","potion_id")
);
--> statement-breakpoint
CREATE TABLE "crafters" (
	"campaign_id" uuid PRIMARY KEY NOT NULL,
	"name" text NOT NULL,
	"class_name" text NOT NULL,
	"level" integer NOT NULL,
	"int_mod" integer NOT NULL,
	"crafting_rank" text NOT NULL,
	"alchemical_crafting" boolean NOT NULL,
	"tools" text NOT NULL,
	"downtime_days" integer DEFAULT 0 NOT NULL
);
--> statement-breakpoint
ALTER TABLE "brews" ADD CONSTRAINT "brews_campaign_id_campaigns_id_fk" FOREIGN KEY ("campaign_id") REFERENCES "public"."campaigns"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "codex_entries" ADD CONSTRAINT "codex_entries_campaign_id_campaigns_id_fk" FOREIGN KEY ("campaign_id") REFERENCES "public"."campaigns"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "crafters" ADD CONSTRAINT "crafters_campaign_id_campaigns_id_fk" FOREIGN KEY ("campaign_id") REFERENCES "public"."campaigns"("id") ON DELETE cascade ON UPDATE no action;