/**
 * Countries and currencies people pick in the rule builder: ISO 3166-1 alpha-2 and ISO 4217
 * codes, named in English by the browser (Intl), sorted by name.
 */

const COUNTRY_CODES =
  "AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW".split(
    " ",
  );

/** Currencies when the browser cannot list them. */
const FALLBACK_CURRENCIES =
  "AED AUD BRL CAD CHF CNY EGP EUR GBP GHS HKD INR JPY KES MAD NGN RWF SAR TZS UGX USD XAF XOF ZAR".split(
    " ",
  );

export interface Option {
  code: string;
  name: string;
}

function named(codes: string[], type: "region" | "currency"): Option[] {
  let names: Intl.DisplayNames | undefined;
  try {
    names = new Intl.DisplayNames(["en"], { type });
  } catch {
    names = undefined;
  }
  return codes
    .map((code) => ({ code, name: names?.of(code) ?? code }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

let countries: Option[] | undefined;
let currencies: Option[] | undefined;

export function countryOptions(): Option[] {
  countries ??= named(COUNTRY_CODES, "region");
  return countries;
}

export function currencyOptions(): Option[] {
  if (!currencies) {
    let codes = FALLBACK_CURRENCIES;
    try {
      codes = Intl.supportedValuesOf("currency");
    } catch {
      // Older browsers: the common currencies.
    }
    currencies = named(codes, "currency");
  }
  return currencies;
}
