import { getCompanies, API_BASE } from "@/lib/api";
import { CompanyCard } from "@/app/components/CompanyCard";
import { AddCompanyForm } from "@/app/components/AddCompanyForm";

export default async function Home() {
  let companies;
  try {
    companies = await getCompanies();
  } catch {
    return <ApiDown />;
  }

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-bold">Watchlist ({companies.length})</h1>
        <AddCompanyForm />
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {companies.map((c) => (
          <CompanyCard key={c.id} company={c} />
        ))}
      </div>
    </div>
  );
}

function ApiDown() {
  return (
    <div className="rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">
      Couldn&apos;t reach the API at <code>{API_BASE}</code>. Start it with <code>make run-api</code>.
    </div>
  );
}
