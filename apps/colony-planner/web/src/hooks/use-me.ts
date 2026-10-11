import { useSuspenseQuery } from "@tanstack/react-query";
import { meQuery } from "@/api/queries";
import type { Me } from "@/lib/access";

/** The signed-in member. Only for screens behind a route guard, which has already loaded it. */
export function useMe(): Me {
	return useSuspenseQuery(meQuery).data;
}
