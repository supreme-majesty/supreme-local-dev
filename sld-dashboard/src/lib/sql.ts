import { createContext, useContext } from "react";

/** The SQL dialect of the database the Database page is connected to. */
export type SqlDriver = "mysql" | "postgres";

/**
 * Provided by the Database page; components that build SQL read it so their
 * statements match the connected server (MySQL quotes names with backticks,
 * PostgreSQL with double quotes, and some statements differ).
 */
export const SqlDriverContext = createContext<SqlDriver>("mysql");

export function useSqlDriver(): SqlDriver {
  return useContext(SqlDriverContext);
}

/** Quotes a table, column or database name for the given dialect. */
export function quoteIdent(driver: SqlDriver, name: string): string {
  if (driver === "postgres") {
    return `"${name.replace(/"/g, '""')}"`;
  }
  return `\`${name.replace(/`/g, "``")}\``;
}

/** Quotes several names and joins them with ", ". */
export function quoteIdents(driver: SqlDriver, names: string[]): string {
  return names.map((n) => quoteIdent(driver, n)).join(", ");
}

/** Quotes a string literal ('it''s'), the same in both dialects. */
export function quoteLiteral(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

/**
 * Column types offered in the table designers, written the way each dialect
 * spells them (PostgreSQL has no DATETIME, LONGTEXT, BLOB or DOUBLE).
 */
export function columnType(driver: SqlDriver, type: string): string {
  if (driver !== "postgres") return type;
  switch (type.toUpperCase()) {
    case "DATETIME":
      return "TIMESTAMP";
    case "LONGTEXT":
      return "TEXT";
    case "BLOB":
      return "BYTEA";
    case "DOUBLE":
      return "DOUBLE PRECISION";
    case "FLOAT":
      return "REAL";
    case "JSON":
      return "JSONB";
    default:
      return type;
  }
}
