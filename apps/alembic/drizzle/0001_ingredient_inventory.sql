CREATE TABLE "ingredient_inventory" (
	"campaign_id" uuid NOT NULL,
	"ingredient_id" text NOT NULL,
	"amount" integer DEFAULT 1 NOT NULL,
	CONSTRAINT "ingredient_inventory_campaign_id_ingredient_id_pk" PRIMARY KEY("campaign_id","ingredient_id"),
	CONSTRAINT "ingredient_inventory_amount_non_negative" CHECK ("ingredient_inventory"."amount" >= 0)
);
--> statement-breakpoint
ALTER TABLE "ingredient_inventory" ADD CONSTRAINT "ingredient_inventory_campaign_id_campaigns_id_fk" FOREIGN KEY ("campaign_id") REFERENCES "public"."campaigns"("id") ON DELETE cascade ON UPDATE no action;