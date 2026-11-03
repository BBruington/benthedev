CREATE TABLE "potion_inventory" (
	"campaign_id" uuid NOT NULL,
	"potion_id" text NOT NULL,
	"amount" integer DEFAULT 0 NOT NULL,
	CONSTRAINT "potion_inventory_campaign_id_potion_id_pk" PRIMARY KEY("campaign_id","potion_id"),
	CONSTRAINT "potion_inventory_amount_non_negative" CHECK ("potion_inventory"."amount" >= 0)
);
--> statement-breakpoint
CREATE TABLE "tradition_knowledge" (
	"campaign_id" uuid NOT NULL,
	"item_kind" text NOT NULL,
	"item_id" text NOT NULL,
	"tradition" text NOT NULL,
	CONSTRAINT "tradition_knowledge_campaign_id_item_kind_item_id_tradition_pk" PRIMARY KEY("campaign_id","item_kind","item_id","tradition"),
	CONSTRAINT "tradition_knowledge_item_kind_valid" CHECK ("tradition_knowledge"."item_kind" in ('ingredient', 'potion')),
	CONSTRAINT "tradition_knowledge_tradition_valid" CHECK ("tradition_knowledge"."tradition" in ('arcane', 'primal', 'occult', 'divine'))
);
--> statement-breakpoint
ALTER TABLE "potion_inventory" ADD CONSTRAINT "potion_inventory_campaign_id_campaigns_id_fk" FOREIGN KEY ("campaign_id") REFERENCES "public"."campaigns"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "tradition_knowledge" ADD CONSTRAINT "tradition_knowledge_campaign_id_campaigns_id_fk" FOREIGN KEY ("campaign_id") REFERENCES "public"."campaigns"("id") ON DELETE cascade ON UPDATE no action;