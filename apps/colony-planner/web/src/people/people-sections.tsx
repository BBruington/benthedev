import { Link } from "@tanstack/react-router";
import { ArrowRightLeftIcon, EyeOffIcon, LogOutIcon, UserPlusIcon, UsersIcon } from "lucide-react";
import type { Schemas } from "@/api/client";
import { ColonyIcon } from "@/components/colony-icon";
import { MemberAvatar } from "@/components/member-avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useCampaign } from "@/hooks/use-campaign";
import { formatNumber } from "@/lib/colony";
import { hiddenTrueName } from "@/lib/people";
import { cn } from "@/lib/utils";
import type { ColonistTarget } from "./colonist-actions";

type Workforce = Schemas["Workforce"];
type NpcReference = Schemas["NpcReference"];

export const roleTableHeadingId = "role-table-heading";
export const namedColonistsHeadingId = "named-colonists-heading";

const numberCell = "px-2 py-3 text-right tabular-nums sm:px-4";

/**
 * Every role's headcount: unnamed + named = total for the DM, and the totals alone for players
 * (spec §5.5). Unassigned stands out while anyone waits there, and the total row is the board's
 * Colonists tile.
 */
export function RoleTable({
	workforce,
	isDm,
	onSetCount,
	onTransferFrom,
}: {
	workforce: Workforce;
	isDm: boolean;
	onSetCount: (roleKey: string) => void;
	onTransferFrom: (roleKey: string) => void;
}) {
	return (
		<section
			aria-labelledby={roleTableHeadingId}
			className="rounded-2xl border border-border bg-card shadow-card"
		>
			<div className="flex items-center gap-3 border-border border-b px-4 py-4 sm:px-6">
				<span
					aria-hidden="true"
					className="inline-flex size-9 shrink-0 items-center justify-center rounded-lg bg-verdigris/14 text-verdigris-ink"
				>
					<UsersIcon className="size-[1.125rem]" />
				</span>
				<h2
					className="font-display font-semibold text-xl tracking-wide outline-none"
					id={roleTableHeadingId}
					tabIndex={-1}
				>
					Roles
				</h2>
			</div>
			<table className="w-full border-collapse text-base">
				<caption className="sr-only">
					{isDm
						? "Colonists by role: unnamed, named and total"
						: "Colonists by role, with each role's total"}
				</caption>
				<thead>
					<tr className="border-border border-b text-muted-foreground text-xs uppercase sm:tracking-[0.12em]">
						<th className="px-4 py-2 text-left font-medium sm:px-6" scope="col">
							Role
						</th>
						{isDm ? (
							<>
								<th className="px-2 py-2 text-right font-medium sm:px-4" scope="col">
									Unnamed
								</th>
								<th className="px-2 py-2 text-right font-medium sm:px-4" scope="col">
									Named
								</th>
							</>
						) : null}
						<th className="px-2 py-2 text-right font-medium sm:px-4" scope="col">
							Total
						</th>
						{isDm ? (
							<th className="hidden w-0 px-2 py-2 sm:table-cell sm:pr-4" scope="col">
								<span className="sr-only">Actions</span>
							</th>
						) : null}
					</tr>
				</thead>
				<tbody>
					{workforce.roles.map((role) => {
						const isUnassignedWaiting = role.isSystem && role.total > 0;
						return (
							<tr
								className={cn(
									"border-border border-b last:border-b-0",
									isUnassignedWaiting && "bg-gilt/10",
								)}
								key={role.key}
							>
								<th className="py-3 pr-2 pl-4 text-left font-normal sm:px-6" scope="row">
									<span
										className={cn(
											"inline-flex items-center gap-2",
											isUnassignedWaiting && "font-semibold text-gilt-ink",
										)}
									>
										<ColonyIcon aria-hidden="true" className="size-4 shrink-0" name={role.icon} />
										<span className="break-words">{role.label}</span>
									</span>
								</th>
								{isDm ? (
									<>
										<td className={numberCell}>{formatNumber(role.unnamedCount)}</td>
										<td className={numberCell}>{formatNumber(role.namedCount)}</td>
									</>
								) : null}
								<td className={cn(numberCell, "font-semibold")}>{formatNumber(role.total)}</td>
								{isDm ? (
									<td className="hidden px-1 py-1 sm:table-cell sm:pr-3">
										<div className="flex justify-end">
											<Button
												aria-label={`Set ${role.label}'s count`}
												onClick={() => {
													onSetCount(role.key);
												}}
												size="icon"
												variant="ghost"
											>
												<UsersIcon />
											</Button>
											<Button
												aria-label={`Transfer from ${role.label}`}
												disabled={role.unnamedCount === 0}
												onClick={() => {
													onTransferFrom(role.key);
												}}
												size="icon"
												variant="ghost"
											>
												<ArrowRightLeftIcon />
											</Button>
										</div>
									</td>
								) : null}
							</tr>
						);
					})}
				</tbody>
				<tfoot>
					<tr className="border-border border-t-2 bg-surface-sunken/40">
						<th className="px-4 py-3 text-left font-semibold sm:px-6" scope="row">
							Colonists
						</th>
						{isDm ? <td className={numberCell} colSpan={2} /> : null}
						<td className={cn(numberCell, "font-bold text-lg")}>
							{formatNumber(workforce.colonists)}
						</td>
						{isDm ? <td className="hidden sm:table-cell" /> : null}
					</tr>
				</tfoot>
			</table>
		</section>
	);
}

type NamedColonistActions = {
	onJoin: () => void;
	onChangeRole: (target: ColonistTarget) => void;
	onLeave: (target: ColonistTarget, roleLabel: string) => void;
};

/** A person's avatar: initials of the name the party uses, since portraits arrive with M4. */
export function NpcAvatar({
	displayName,
	size = "md",
}: {
	displayName: string;
	size?: "sm" | "md" | "lg" | "xl";
}) {
	return <MemberAvatar avatarUrl={null} name={displayName} size={size} />;
}

/** Marks a row players can't see, in the DM's view (spec §5.9); players never receive hidden rows. */
export function HiddenBadge({ item }: { item: { dm?: { revealed: boolean } } }) {
	if (!item.dm || item.dm.revealed) {
		return null;
	}
	return (
		<Badge variant="muted">
			<EyeOffIcon />
			Hidden
		</Badge>
	);
}

/** The name the party uses, linking to their page, with the true name under it for the DM. */
export function NpcNameLink({ npc }: { npc: NpcReference }) {
	const { slug } = useCampaign();
	const trueName = hiddenTrueName(npc);
	return (
		<span className="grid">
			<Link
				className="font-semibold underline-offset-4 [overflow-wrap:anywhere] hover:underline"
				params={{ campaignSlug: slug, npcId: npc.id }}
				to="/c/$campaignSlug/npcs/$npcId"
			>
				{npc.displayName}
			</Link>
			{trueName ? (
				<span className="text-muted-foreground text-sm [overflow-wrap:anywhere]">
					True name {trueName}
				</span>
			) : null}
		</span>
	);
}

/**
 * The colonists with names, by role. Players see only revealed ones (the server leaves the rest
 * out); hidden ones still count in the table above.
 */
export function NamedColonists({
	workforce,
	actions,
}: {
	workforce: Workforce;
	/** The DM's controls; null for players. */
	actions: NamedColonistActions | null;
}) {
	const colonists = workforce.roles.flatMap((role) =>
		role.namedColonists.map((colonist) => ({ colonist, role })),
	);
	return (
		<section aria-labelledby={namedColonistsHeadingId} className="flex flex-col gap-3">
			<div className="flex flex-wrap items-end justify-between gap-3">
				<h2
					className="font-display font-semibold text-2xl tracking-wide outline-none"
					id={namedColonistsHeadingId}
					tabIndex={-1}
				>
					Named colonists
				</h2>
				{actions ? (
					<Button onClick={actions.onJoin} variant="outline">
						<UserPlusIcon />
						Join colony
					</Button>
				) : null}
			</div>
			{colonists.length === 0 ? (
				<p className="rounded-xl border border-border border-dashed px-4 py-6 text-center text-muted-foreground">
					{actions
						? "No NPC has joined the colony yet. Join one here or from their page."
						: "Nobody the party knows by name has joined the colony yet."}
				</p>
			) : (
				<ul className="grid gap-2 md:grid-cols-2">
					{colonists.map(({ colonist, role }) => {
						const target: ColonistTarget = {
							displayName: colonist.displayName,
							id: colonist.id,
							roleKey: role.key,
						};
						return (
							<li
								className="flex flex-wrap items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 shadow-card"
								key={colonist.id}
							>
								<NpcAvatar displayName={colonist.displayName} />
								<div className="min-w-0 flex-1">
									<p className="flex flex-wrap items-center gap-2">
										<NpcNameLink npc={colonist} />
										<HiddenBadge item={colonist} />
									</p>
									<p className="inline-flex items-center gap-1.5 text-muted-foreground text-sm">
										<ColonyIcon aria-hidden="true" className="size-4" name={role.icon} />
										{role.label}
									</p>
								</div>
								{actions ? (
									<div className="flex gap-1">
										<Button
											aria-label={`Change ${colonist.displayName}'s role`}
											onClick={() => {
												actions.onChangeRole(target);
											}}
											size="sm"
											variant="ghost"
										>
											<ArrowRightLeftIcon />
											Role
										</Button>
										<Button
											aria-label={`${colonist.displayName} leaves the colony`}
											onClick={() => {
												actions.onLeave(target, role.label);
											}}
											size="sm"
											variant="ghost-destructive"
										>
											<LogOutIcon />
											Leave
										</Button>
									</div>
								) : null}
							</li>
						);
					})}
				</ul>
			)}
		</section>
	);
}
