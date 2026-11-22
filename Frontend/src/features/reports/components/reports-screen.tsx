"use client";

import { useId, useMemo, useState } from "react";

import {
  DEFAULT_ZONE,
  GROUPINGS,
  NO_FILTERS,
  PERIOD_OPTIONS,
  civilToday,
  draftOf,
  filtersOf,
  firstOfMonth,
  resolvePeriod,
  type ReportDimension,
  type ReportFilters,
  type ReportPeriod,
  type SavedReport,
} from "@/entities/report/model";
import { ReportResults } from "@/features/reports/components/report-results";
import { SavedReportsPanel } from "@/features/reports/components/saved-reports-panel";
import { useFilterOptions, type Option } from "@/features/reports/use-filter-options";
import { useSavedReports } from "@/features/reports/use-saved-reports";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { usePractice, useWritesLocked } from "@/shared/auth/practice";
import { getSessionUser } from "@/shared/auth/session";
import { Button } from "@/shared/ui/button";
import { Field, inputClass } from "@/shared/ui/field";
import { Segmented } from "@/shared/ui/segmented";

/**
 * The reports page (T60): the billing report with its period, cut and filters chosen by the
 * person, the CSV of what is on screen, and the reports the practice saves and shares.
 *
 * What is saved is the question (period, cut, filters), never the figures. A relative period
 * such as "este mes" is saved as that, so opening the report next month answers for next month.
 * Anyone who reads reports may save one; only its author, the owner and the admin change or
 * remove it. A professional still reads only their own figures, whatever report they open.
 */
export function ReportsScreen() {
  const role = getSessionUser()?.role;
  const ownOnly = role === "professional";
  const { status } = usePractice();
  const zone = status?.timezone || DEFAULT_ZONE;
  const today = useMemo(() => civilToday(zone), [zone]);
  const writesLocked = useWritesLocked();
  const prefix = useId();

  const [period, setPeriod] = useState<ReportPeriod>("this-month");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [groupBy, setGroupBy] = useState<ReportDimension>("month");
  const [filters, setFilters] = useState<ReportFilters>(NO_FILTERS);
  // A saved report may name a professional or a mutual the lists no longer offer (a deactivated
  // one): its name travels with it so the select can still show what is selected.
  const [extraOptions, setExtraOptions] = useState<Record<string, string>>({});

  const [active, setActive] = useState<SavedReport | null>(null);
  const [name, setName] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const { professionals, insurers } = useFilterOptions(role);
  const saved = useSavedReports();

  const resolved = resolvePeriod(period, from, to, today);

  function choosePeriod(next: ReportPeriod) {
    setPeriod(next);
    if (next === "custom" && from === "" && to === "") {
      setFrom(firstOfMonth(today));
      setTo(today);
    }
  }

  function changeFilters(patch: Partial<ReportFilters>) {
    setFilters((current) => {
      const next = { ...current, ...patch };
      // A mutual only means something for patients who have one.
      if (next.coverage === "private") next.insurerId = "";
      return next;
    });
  }

  function open(report: SavedReport) {
    setPeriod(report.period);
    setFrom(report.from ?? "");
    setTo(report.to ?? "");
    setGroupBy(report.groupBy);
    setFilters(filtersOf(report));
    setExtraOptions({
      ...(report.professionalId ? { [report.professionalId]: report.professionalName ?? "" } : {}),
      ...(report.insurerId ? { [report.insurerId]: report.insurerName ?? "" } : {}),
    });
    setActive(report);
    setName(report.name);
    setSaveError(null);
  }

  async function save(mode: "create" | "update") {
    if (!resolved.ok) return;
    setSaveError(null);
    if (name.trim() === "") {
      setSaveError("Escribí un nombre para el informe.");
      return;
    }

    const body = draftOf(name, period, from, to, groupBy, filters);
    setSaving(true);
    try {
      const result =
        mode === "update" && active !== null
          ? await browserApi.PUT("/api/v1/reports/saved/{id}", {
              params: { path: { id: active.id } },
              body,
            })
          : await browserApi.POST("/api/v1/reports/saved", { body });
      if (!result.response.ok || result.data === undefined) {
        setSaveError(apiErrorMessage(result.error));
        return;
      }
      await saved.reload();
      // The write answers without the names the list adds, so the open report is the listed one.
      setActive({ ...result.data, ...nameOf(result.data, professionals, insurers, extraOptions) });
    } catch {
      setSaveError("No pudimos contactar al servidor.");
    } finally {
      setSaving(false);
    }
  }

  const canUpdate = active !== null && active.canManage;

  return (
    <>
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Informes</h1>
        <p className="mt-1 text-sm text-ink-muted">
          {ownOnly
            ? "Tus turnos y comprobantes, con el período, el corte y los filtros que elijas."
            : "Lo facturado y lo atendido sin facturar, con el período, el corte y los filtros que elijas. Guardá los que repitas para todo el consultorio."}
        </p>
      </header>

      <div className="mt-6 grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="grid min-w-0 content-start gap-4">
          <section
            aria-label="Período, corte y filtros"
            className="grid gap-4 rounded-panel border border-border bg-surface p-5"
          >
            <div className="flex flex-wrap items-end gap-x-6 gap-y-3">
              <div>
                <p className="text-xs font-medium text-ink-muted">Período</p>
                <div className="mt-1">
                  <Segmented
                    label="Período"
                    options={PERIOD_OPTIONS}
                    value={period}
                    onChange={choosePeriod}
                  />
                </div>
              </div>
              {period === "custom" ? (
                <>
                  <Field label="Desde" htmlFor={`${prefix}-from`}>
                    <input
                      id={`${prefix}-from`}
                      type="date"
                      value={from}
                      onChange={(event) => setFrom(event.target.value)}
                      className={inputClass}
                    />
                  </Field>
                  <Field label="Hasta" htmlFor={`${prefix}-to`}>
                    <input
                      id={`${prefix}-to`}
                      type="date"
                      value={to}
                      onChange={(event) => setTo(event.target.value)}
                      className={inputClass}
                    />
                  </Field>
                </>
              ) : null}
              <div>
                <p className="text-xs font-medium text-ink-muted">Cortar por</p>
                <div className="mt-1">
                  <Segmented label="Cortar por" options={GROUPINGS} value={groupBy} onChange={setGroupBy} />
                </div>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              {ownOnly ? null : (
                <FilterSelect
                  id={`${prefix}-professional`}
                  label="Profesional"
                  allLabel="Todos"
                  value={filters.professionalId}
                  options={withCurrent(professionals, filters.professionalId, extraOptions)}
                  onChange={(professionalId) => changeFilters({ professionalId })}
                />
              )}
              <Field label="Cobertura" htmlFor={`${prefix}-coverage`}>
                <select
                  id={`${prefix}-coverage`}
                  value={filters.coverage}
                  onChange={(event) =>
                    changeFilters({ coverage: event.target.value as ReportFilters["coverage"] })
                  }
                  className={inputClass}
                >
                  <option value="">Todas</option>
                  <option value="private">Particular</option>
                  <option value="insurer">Con mutual</option>
                </select>
              </Field>
              {filters.coverage === "private" ? null : (
                <FilterSelect
                  id={`${prefix}-insurer`}
                  label="Mutual"
                  allLabel="Todas"
                  value={filters.insurerId}
                  options={withCurrent(insurers, filters.insurerId, extraOptions)}
                  onChange={(insurerId) => changeFilters({ insurerId })}
                />
              )}
              <Field label="Modalidad" htmlFor={`${prefix}-modality`}>
                <select
                  id={`${prefix}-modality`}
                  value={filters.modality}
                  onChange={(event) =>
                    changeFilters({ modality: event.target.value as ReportFilters["modality"] })
                  }
                  className={inputClass}
                >
                  <option value="">Todas</option>
                  <option value="presencial">Presencial</option>
                  <option value="online">Online</option>
                </select>
              </Field>
            </div>

            {!resolved.ok ? (
              <p role="alert" className="text-sm text-danger">
                {resolved.message}
              </p>
            ) : null}
          </section>

          {resolved.ok ? (
            <ReportResults range={resolved.range} groupBy={groupBy} filters={filters} />
          ) : null}
        </div>

        <aside className="grid min-w-0 content-start gap-4">
          <section
            aria-labelledby={`${prefix}-save-title`}
            className="rounded-panel border border-border bg-surface p-5"
          >
            <h2 id={`${prefix}-save-title`} className="text-sm font-semibold">
              {active ? "Informe abierto" : "Guardar este informe"}
            </h2>
            <p className="mt-1 text-xs text-ink-muted">
              {active
                ? `Abriste «${active.name}». Cambiá lo que quieras y actualizalo, o guardá una copia con otro nombre.`
                : "Guarda el período, el corte y los filtros con un nombre, para todo el consultorio."}
            </p>
            <form
              className="mt-3 grid gap-3"
              onSubmit={(event) => {
                event.preventDefault();
                void save(canUpdate ? "update" : "create");
              }}
            >
              <Field label="Nombre" htmlFor={`${prefix}-name`} error={saveError ?? undefined}>
                <input
                  id={`${prefix}-name`}
                  type="text"
                  value={name}
                  maxLength={80}
                  onChange={(event) => setName(event.target.value)}
                  aria-invalid={saveError !== null || undefined}
                  className={inputClass}
                />
              </Field>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  busy={saving}
                  disabled={writesLocked || !resolved.ok}
                >
                  {canUpdate ? "Actualizar informe" : "Guardar informe"}
                </Button>
                {canUpdate ? (
                  <Button
                    size="sm"
                    busy={saving}
                    disabled={writesLocked || !resolved.ok}
                    onClick={() => void save("create")}
                  >
                    Guardar como nuevo
                  </Button>
                ) : null}
                {active ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setActive(null);
                      setName("");
                      setSaveError(null);
                    }}
                  >
                    Soltar
                  </Button>
                ) : null}
              </div>
            </form>
          </section>

          <SavedReportsPanel
            reports={saved.reports}
            failed={saved.failed}
            activeId={active?.id ?? null}
            writesLocked={writesLocked}
            onOpen={open}
            onRemoved={(removed) => {
              if (active?.id === removed.id) {
                setActive(null);
                setName("");
              }
              void saved.reload();
            }}
          />
        </aside>
      </div>
    </>
  );
}

/** The names of what a report filters by, from the lists or from what was already open. */
function nameOf(
  report: SavedReport,
  professionals: readonly Option[],
  insurers: readonly Option[],
  extra: Record<string, string>,
): Pick<SavedReport, "professionalName" | "insurerName"> {
  const find = (list: readonly Option[], id: string | undefined) =>
    id ? (list.find((option) => option.id === id)?.name ?? extra[id] ?? "") : "";
  return {
    professionalName: find(professionals, report.professionalId),
    insurerName: find(insurers, report.insurerId),
  };
}

/** The options of a list, plus the selected one when the list no longer has it. */
function withCurrent(
  list: readonly Option[],
  current: string,
  extra: Record<string, string>,
): Option[] {
  if (current === "" || list.some((option) => option.id === current)) return [...list];
  return [...list, { id: current, name: extra[current] || "Elegido en el informe" }];
}

function FilterSelect({
  id,
  label,
  allLabel,
  value,
  options,
  onChange,
}: {
  id: string;
  label: string;
  allLabel: string;
  value: string;
  options: readonly Option[];
  onChange: (value: string) => void;
}) {
  return (
    <Field label={label} htmlFor={id}>
      <select
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className={inputClass}
      >
        <option value="">{allLabel}</option>
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            {option.name}
          </option>
        ))}
      </select>
    </Field>
  );
}
