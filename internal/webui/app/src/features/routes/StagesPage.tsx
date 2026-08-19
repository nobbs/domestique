/**
 * The route library: a grid of stage cards, each linking to its full preview.
 * A later feature adds a sibling directory here rather than changing this one.
 *
 * The search and the order live here rather than in the controls, because they
 * describe this grid: the controls state them and hand back a change, and one
 * place decides what is shown. Neither reaches the service — the listing is
 * already held, and narrowing it is a question about what is on screen.
 */

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { stagesQuery } from "../../api/queries";
import { stageKey } from "../../api/types";
import { Button } from "../../components/Button";
import { Layout } from "../../components/Layout";
import { RouteGrid } from "../../components/RouteCard";
import { ErrorMessage, LoadingMessage, StatusMessage } from "../../components/StatusMessage";
import type { StageSort } from "../../lib/library";
import { arrangeStages, stageCounts } from "../../lib/library";
import { SyncControls } from "../sync/SyncControls";
import { TargetConvergence } from "../sync/TargetConvergence";
import { LibraryControls } from "./LibraryControls";
import { MapAttribution } from "./MapAttribution";
import { StageCard } from "./StageCard";
import { SyncStatusBadge } from "./SyncStatusBadge";

export function StagesPage() {
  const { data, isPending, isError, error } = useQuery(stagesQuery());
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<StageSort>("name");

  const shown = useMemo(() => arrangeStages(data ?? [], query, sort), [data, query, sort]);
  // Over the whole library rather than what the search left, so a card keeps
  // saying which of its route's stages it is however the grid is narrowed.
  const counts = useMemo(() => stageCounts(data ?? []), [data]);

  return (
    <Layout status={<SyncStatusBadge />}>
      {/*
       * The page's own name. Every other heading on it is a section heading, so
       * without this the document starts at level two and a reader navigating by
       * heading is dropped into the middle of a hierarchy with no top. It is not
       * drawn, because the header beside it already says whose library this is
       * and a second title under the wordmark would be the same word twice.
       */}
      <h1 className="visually-hidden">Route library</h1>
      <SyncControls />
      <TargetConvergence />
      {isPending ? <LoadingMessage what="the route library" /> : null}
      {isError ? <ErrorMessage what="the route library" error={error} /> : null}
      {data && data.length === 0 ? (
        <StatusMessage
          title="No routes yet"
          detail="Stages appear here after the first successful synchronisation."
        />
      ) : null}
      {data && data.length > 0 ? (
        <>
          <LibraryControls
            query={query}
            onQueryChange={setQuery}
            sort={sort}
            onSortChange={setSort}
            shown={shown.length}
            total={data.length}
          />
          {shown.length === 0 ? (
            // An empty grid under a search box is ambiguous: a library with
            // nothing in it and a search that matched nothing look the same. The
            // way back is offered as an action rather than left to the reader to
            // find, because the search that emptied the grid is also what hid
            // everything they could have clicked instead.
            <StatusMessage
              title={`No stages match “${query.trim()}”.`}
              detail="Search matches route and stage names."
            >
              <Button variant="quiet" onClick={() => setQuery("")}>
                Clear search
              </Button>
            </StatusMessage>
          ) : (
            <>
              <RouteGrid>
                {shown.map((stage) => (
                  <StageCard
                    key={stageKey(stage)}
                    stage={stage}
                    stageCount={counts.get(stage.routeId)}
                  />
                ))}
              </RouteGrid>
              <MapAttribution />
            </>
          )}
        </>
      ) : null}
    </Layout>
  );
}
