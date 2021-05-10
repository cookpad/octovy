import React from "react";

import Paper from "@material-ui/core/Paper";
import Toolbar from "@material-ui/core/Toolbar";
import Tooltip from "@material-ui/core/Tooltip";
import AppBar from "@material-ui/core/AppBar";
import Grid from "@material-ui/core/Grid";
import IconButton from "@material-ui/core/IconButton";
import RefreshIcon from "@material-ui/icons/Refresh";
import TextField from "@material-ui/core/TextField";
import Checkbox from "@material-ui/core/Checkbox";

import Table from "@material-ui/core/Table";
import TableBody from "@material-ui/core/TableBody";
import TableCell from "@material-ui/core/TableCell";
import TableContainer from "@material-ui/core/TableContainer";
import TableHead from "@material-ui/core/TableHead";
import TableRow from "@material-ui/core/TableRow";

import Chip from "@material-ui/core/Chip";

import { useParams } from "react-router-dom";
import useStyles from "./style";
import { Link as RouterLink } from "react-router-dom";

import * as model from "./model";

interface packageStatus {
  isLoaded: boolean;
  result?: model.scanResult;
  displayed: model.packageSource[];
}

export default function Repository() {
  const classes = useStyles();

  const { owner, repoName } = useParams();
  const [branch, setBranch] = React.useState<string>();
  const [branchInput, setBranchInput] = React.useState<string>("");
  const [vulnFilter, setVulnFilter] = React.useState<boolean>(true);

  const [pkgStatus, setPkgStatus] = React.useState<packageStatus>({
    isLoaded: false,
    result: undefined,
    displayed: [],
  });
  const [err, setErr] = React.useState("");

  const getRepositoryInfo = () => {
    fetch(`api/v1/repo/${owner}/${repoName}`)
      .then((res) => res.json())
      .then(
        (result) => {
          console.log(result);
          setBranch(result.data.DefaultBranch);
        },
        (error) => {
          setErr(error);
        }
      );
  };

  const updatePackages = () => {
    console.log("update:", { branch });
    if (branch === undefined) {
      return;
    }
    setBranchInput(branch);

    fetch(`api/v1/scan/${owner}/${repoName}/${branch}/result`)
      .then((res) => res.json())
      .then(
        (result) => {
          console.log(result);
          setPkgStatus({
            isLoaded: true,
            result: result.data,
            displayed: filterPackages(result.data.Sources),
          });
        },
        (error) => {
          setErr(error);
        }
      );
  };

  const filterPackages = (
    sources: model.packageSource[]
  ): model.packageSource[] => {
    if (sources === undefined) {
      return [];
    }

    return sources.map((src) => {
      return {
        Source: src.Source,
        Packages: src.Packages.filter((pkg) => {
          return !vulnFilter || pkg.Vulnerabilities.length > 0;
        }),
      };
    });
  };

  const updateVulnFilter = () => {
    if (!pkgStatus.result) {
      return;
    }

    setPkgStatus({
      isLoaded: pkgStatus.isLoaded,
      result: pkgStatus.result,
      displayed: filterPackages(pkgStatus.result.Sources),
    });
  };

  const onKeyUpBranch = (e: any) => {
    if (e.which === 13) {
      setBranch(e.target.value);
    }
  };
  const onChangeBranch = (e: any) => {
    setBranchInput(e.target.value);
  };
  const onChangeVulnFilter = (event: React.ChangeEvent<HTMLInputElement>) => {
    setVulnFilter(event.target.checked);
  };

  const packageView = () => {
    if (!pkgStatus.isLoaded) {
      return <div className={classes.contentWrapper}>Loading...</div>;
    } else if (Object.keys(pkgStatus.displayed).length === 0) {
      return <div>No data</div>;
    } else {
      console.log({ pkgStatus });
      return (
        <div>
          {pkgStatus.displayed.map((src, idx) => {
            return (
              <div key={idx}>
                <Grid item xs={12}>
                  <Grid component="h4"> {src.Source} </Grid>
                </Grid>

                <TableContainer component={Paper}>
                  <Table
                    className={classes.packageTable}
                    size="small"
                    aria-label="simple table">
                    <TableHead className={classes.packageTableHeader}>
                      <TableRow>
                        <TableCell className={classes.packageTableNameRow}>
                          Name
                        </TableCell>
                        <TableCell className={classes.packageTableVersionRow}>
                          Version
                        </TableCell>
                        <TableCell className={classes.packageTableVulnRow}>
                          Vulnerabilities
                        </TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {src.Packages.map((pkg, idx) => (
                        <TableRow key={idx}>
                          <TableCell component="th" scope="row">
                            <RouterLink
                              to={`/package?name=${pkg.Name}&type=${pkg.Type}`}>
                              {pkg.Name}
                            </RouterLink>
                          </TableCell>
                          <TableCell>{pkg.Version}</TableCell>
                          <TableCell className={classes.packageTableVulnCell}>
                            {pkg.Vulnerabilities.map((vulnID, idx) => {
                              return (
                                <Chip
                                  component={RouterLink}
                                  to={"/vuln/" + vulnID}
                                  key={idx}
                                  size="small"
                                  label={vulnID}
                                  color="secondary"
                                  clickable
                                />
                              );
                            })}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              </div>
            );
          })}
        </div>
      );
    }
  };

  React.useEffect(getRepositoryInfo, []);
  React.useEffect(updatePackages, [branch]);
  React.useEffect(updateVulnFilter, [vulnFilter]);

  return (
    <Paper className={classes.paper}>
      <AppBar position="static" color="default" elevation={1}>
        <Toolbar>
          <Grid container spacing={3} alignItems="center">
            <Grid component="h3">
              {owner}/{repoName}
            </Grid>

            <Grid item xs>
              <TextField
                value={branchInput}
                onChange={onChangeBranch}
                onKeyUp={onKeyUpBranch}
                InputProps={{
                  className: classes.searchInput,
                }}
              />
            </Grid>

            <Grid item>
              <Checkbox
                checked={vulnFilter}
                onChange={onChangeVulnFilter}
                inputProps={{ "aria-label": "primary checkbox" }}
              />
              Only vulnerables
            </Grid>

            <Tooltip title="Reload">
              <IconButton onClick={updatePackages}>
                <RefreshIcon className={classes.block} color="inherit" />
              </IconButton>
            </Tooltip>
          </Grid>
        </Toolbar>
      </AppBar>

      <div className={classes.contentWrapper}>{packageView()}</div>
    </Paper>
  );
}
