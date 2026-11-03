import { auth } from "@clerk/tanstack-react-start/server";
import { ORPCError, os } from "@orpc/server";
import {
	brewAttemptSchema,
	crafterEditSchema,
	deconstructSchema,
	inventoryAdjustSchema,
	revealSchema,
	viewingSetSchema,
} from "#/orpc/schema";
import {
	adjustInventory,
	attemptBrew,
	deconstructItem,
	getAnswerKey,
	getWorkshop,
	revealPotion,
	setViewing,
	updateCrafter,
} from "#/server/workshop";

const authenticated = os
	.$context<{ headers: Headers }>()
	.use(async ({ next }) => {
		const { userId } = await auth();
		if (!userId) throw new ORPCError("UNAUTHORIZED");
		return next({ context: { userId } });
	});

export default {
	workshop: {
		get: authenticated.handler(({ context }) => getWorkshop(context.userId)),
	},
	brew: {
		attempt: authenticated
			.input(brewAttemptSchema)
			.handler(({ context, input }) => attemptBrew(context.userId, input)),
	},
	crafter: {
		update: authenticated
			.input(crafterEditSchema)
			.handler(({ context, input }) => updateCrafter(context.userId, input)),
	},
	inventory: {
		adjust: authenticated
			.input(inventoryAdjustSchema)
			.handler(({ context, input }) => adjustInventory(context.userId, input)),
	},
	deconstruct: {
		attempt: authenticated
			.input(deconstructSchema)
			.handler(({ context, input }) => deconstructItem(context.userId, input)),
	},
	viewing: {
		set: authenticated
			.input(viewingSetSchema)
			.handler(({ context, input }) => setViewing(context.userId, input.as)),
	},
	gm: {
		answerKey: authenticated.handler(({ context }) =>
			getAnswerKey(context.userId),
		),
		reveal: authenticated
			.input(revealSchema)
			.handler(({ context, input }) =>
				revealPotion(context.userId, input.potionId, input.reveal),
			),
	},
};
