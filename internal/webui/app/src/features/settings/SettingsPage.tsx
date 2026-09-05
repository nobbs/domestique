import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel, FieldLegend, FieldSet } from "@/components/ui/field";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { PageShell } from "../../components/Layout";
import { THEME_CHOICES, type ThemeChoice } from "../../lib/theme";
import { DataSources } from "./DataSources";
import { WahooAccountCard } from "./WahooAccountCard";

const THEME_LABELS: Record<ThemeChoice, string> = {
  system: "System",
  light: "Light",
  dark: "Dark",
};

export interface SettingsPageProps {
  themeChoice: ThemeChoice;
  onThemeChoiceChange: (choice: ThemeChoice) => void;
}

export function SettingsPage({ themeChoice, onThemeChoiceChange }: SettingsPageProps) {
  return (
    <PageShell>
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-6">
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <Card className="border-[var(--rule)] bg-[var(--panel)] shadow-[var(--shadow)]">
          <CardHeader>
            <CardTitle role="heading" aria-level={2}>
              Display preferences
            </CardTitle>
          </CardHeader>
          <CardContent className="grid gap-8">
            <FieldSet>
              <FieldLegend>Theme</FieldLegend>
              <RadioGroup
                value={themeChoice}
                onValueChange={(value) => {
                  if (THEME_CHOICES.includes(value as ThemeChoice)) {
                    onThemeChoiceChange(value as ThemeChoice);
                  }
                }}
              >
                <FieldGroup>
                  {THEME_CHOICES.map((choice) => (
                    <FieldLabel key={choice}>
                      <Field orientation="horizontal">
                        <RadioGroupItem value={choice} />
                        <span>{THEME_LABELS[choice]}</span>
                      </Field>
                    </FieldLabel>
                  ))}
                </FieldGroup>
              </RadioGroup>
            </FieldSet>
          </CardContent>
        </Card>
        {/*
         * Below the browser's own preferences: the one thing this page holds
         * that is not local, and the one thing here that is this rider's own
         * rather than the whole service's.
         */}
        <WahooAccountCard />
        {/*
         * Last, because it is reference rather than a setting: nothing here is
         * changed, and every credit this service owes is read from one place.
         */}
        <DataSources />
      </div>
    </PageShell>
  );
}
